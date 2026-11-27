// Package agent 实现代理登录、出卡与额度/返点（design.md 第 11 节）。
package agent

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/langyannb/devour-auth/internal/apierr"
	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/card"
	"github.com/langyannb/devour-auth/internal/store"
)

const (
	SessionTTL  = 12 * time.Hour
	idleTimeout = 30 * time.Minute
	maxBatchQty = card.MaxBatchQty
)

type Service struct {
	st    *store.Store
	cards *card.Service
	now   func() time.Time
}

func New(st *store.Store) *Service {
	return &Service{st: st, cards: card.New(st), now: time.Now}
}

var (
	ErrQuota = errors.New("超出本月额度")
)

// ---------- 登录 ----------

type LoginResult struct {
	Token     string
	CSRF      string
	ExpiresAt int64
	Agent     store.Agent
}

func (s *Service) Login(ctx context.Context, name, password, totp, ip, ua string) (LoginResult, error) {
	now := s.now()
	a, err := s.st.AgentByName(ctx, name)
	if errors.Is(err, store.ErrNotFound) {
		return LoginResult{}, apierr.New(apierr.Unauthorized, "账号或密码错误")
	} else if err != nil {
		return LoginResult{}, err
	}
	if !auth.VerifyPassword(password, a.PassHash) {
		return LoginResult{}, apierr.New(apierr.Unauthorized, "账号或密码错误")
	}
	if a.Status != "active" {
		return LoginResult{}, apierr.New(apierr.Unauthorized, "账号已冻结")
	}
	if a.TotpSecret != "" {
		key, err := auth.DecodeTOTPSecret(a.TotpSecret)
		if err != nil || !auth.ValidateTOTP(key, totp, now) {
			return LoginResult{}, apierr.New(apierr.Unauthorized, "动态验证码错误")
		}
	}

	token, err := auth.NewToken()
	if err != nil {
		return LoginResult{}, err
	}
	csrf, err := auth.NewCSRF()
	if err != nil {
		return LoginResult{}, err
	}
	expires := now.Add(SessionTTL).Unix()
	id, err := s.st.CreateAgentSession(ctx, a.ID, auth.HashToken(token), csrf, ip, ua, now.Unix(), expires)
	if err != nil {
		return LoginResult{}, err
	}
	_ = s.st.RevokeOtherAgentSessions(ctx, a.ID, id, now.Unix())

	return LoginResult{Token: token, CSRF: csrf, ExpiresAt: expires, Agent: a}, nil
}

func (s *Service) Logout(ctx context.Context, sessionID int64) error {
	return s.st.RevokeAgentSession(ctx, sessionID, s.now().Unix())
}

// ---------- 出卡 ----------

type IssueReq struct {
	AgentID  int64
	PolicyID int64
	Qty      int
	Prefix   string
	IP       string
}

type IssueResult struct {
	BatchID int64
	Kamis   []string
	Payable int64
	Rebate  int64
}

// PayableCents 代理应付 = 卡类型售价 × 折扣 × 数量。
func PayableCents(priceCents, discountBP int64, qty int) int64 {
	return priceCents * discountBP * int64(qty) / 10000
}

func (s *Service) Issue(ctx context.Context, req IssueReq) (IssueResult, error) {
	if req.Qty < 1 || req.Qty > maxBatchQty {
		return IssueResult{}, apierr.New(apierr.BadInput, "单批数量需在 1..%d", maxBatchQty)
	}
	now := s.now()

	a, err := s.st.AgentByID(ctx, req.AgentID)
	if err != nil {
		return IssueResult{}, err
	}
	if a.Status != "active" {
		return IssueResult{}, apierr.New(apierr.Unauthorized, "账号已冻结")
	}
	p, err := s.st.PolicyByID(ctx, req.PolicyID)
	if err != nil {
		return IssueResult{}, err
	}
	if p.Active != 1 {
		return IssueResult{}, apierr.New(apierr.BadInput, "卡类型不可用")
	}

	payable := PayableCents(p.PriceCents, a.DiscountBP, req.Qty)

	// 额度：本月已出卡金额 + 本次应付 不得超过 monthly_quota_cents（0 = 不限）
	if a.MonthlyQuotaCents > 0 {
		tx, err := s.st.DB().BeginTx(ctx, nil)
		if err != nil {
			return IssueResult{}, err
		}
		used, qerr := s.st.MonthlyIssuedCentsTx(ctx, tx, a.ID, monthStart(now))
		tx.Rollback()
		if qerr != nil {
			return IssueResult{}, qerr
		}
		if used+payable > a.MonthlyQuotaCents {
			return IssueResult{}, fmt.Errorf("%w: 本月已用 %d 分，额度 %d 分", ErrQuota, used, a.MonthlyQuotaCents)
		}
	}

	var parent store.Agent
	if a.ParentID.Valid {
		if parent, err = s.st.AgentByID(ctx, a.ParentID.Int64); err != nil {
			return IssueResult{}, err
		}
	}
	rebate := payable * parent.RebateBP / 10000

	res, err := s.cards.Generate(ctx, card.GenerateReq{
		AppID:     p.AppID,
		PolicyID:  p.ID,
		Qty:       req.Qty,
		Prefix:    req.Prefix,
		AgentID:   a.ID,
		CreatedBy: a.ID,
		Actor:     fmt.Sprintf("agent:%d", a.ID),
		IP:        req.IP,
		Hook: func(ctx context.Context, tx *sql.Tx, batchID int64) error {
			ref := fmt.Sprintf("batch:%d", batchID)
			if err := s.st.AddAgentLedgerTx(ctx, tx, a.ID, "issue", payable, ref, now.Unix()); err != nil {
				return err
			}
			if a.ParentID.Valid && rebate > 0 {
				if err := s.st.AddAgentLedgerTx(ctx, tx, parent.ID, "rebate", -rebate, ref, now.Unix()); err != nil {
					return err
				}
			}
			return nil
		},
	})
	if err != nil {
		return IssueResult{}, err
	}
	return IssueResult{BatchID: res.BatchID, Kamis: res.Kamis, Payable: payable, Rebate: rebate}, nil
}

// monthStart 当月 1 日 00:00（本地时区，账单按月统计）。
func monthStart(t time.Time) int64 {
	return time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, t.Location()).Unix()
}

// VisibleAgentIDs 代理可见范围：自己 + 下级。
func (s *Service) VisibleAgentIDs(ctx context.Context, agentID int64) ([]int64, error) {
	return s.st.ChildAgentIDs(ctx, agentID)
}

// ---------- 账单 ----------

// GenerateBills 汇总上月 issue 流水，生成 open 账单（每月 1 日跑）。
func (s *Service) GenerateBills(ctx context.Context, month string) (int, error) {
	loc := s.now().Location()
	t, err := time.ParseInLocation("2006-01", month, loc)
	if err != nil {
		return 0, fmt.Errorf("月份格式应为 YYYY-MM: %w", err)
	}
	start := t.Unix()
	end := t.AddDate(0, 1, 0).Unix()

	agents, err := s.st.ListAgents(ctx, 0)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, a := range agents {
		ok, err := s.st.UpsertAgentBill(ctx, a.ID, month, start, end)
		if err != nil {
			return n, err
		}
		if ok {
			n++
		}
	}
	return n, nil
}

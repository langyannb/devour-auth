package httpapi_test

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"

	"github.com/langyannb/devour-auth/internal/auth"
	"github.com/langyannb/devour-auth/internal/store"
)

// 导出必须拉全量，不能被单页 200 条截断；ref 含逗号要按 CSV 规则转义。
func TestAgentLedgerExportAllPages(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	hash, err := auth.HashPassword(testPass)
	if err != nil {
		t.Fatal(err)
	}
	id, err := e.st.CreateAgent(ctx, store.Agent{
		Name: "a1", Level: 2, QuotaCents: -1, PassHash: hash, Status: "active", CreatedAt: 1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	tx, err := e.st.DB().BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 250; i++ {
		if err := e.st.AddAgentLedgerTx(ctx, tx, id, "adjust", 100, "t,1", 1000+int64(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	body := fmt.Sprintf(`{"name":"a1","password":%q}`, testPass)
	resp, err := c.Post(e.ts.URL+"/agent/api/login", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("代理登录 = %d", resp.StatusCode)
	}

	resp, err = c.Get(e.ts.URL + "/agent/api/ledger/export")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("导出 = %d", resp.StatusCode)
	}
	rows, err := csv.NewReader(resp.Body).ReadAll()
	if err != nil {
		t.Fatalf("CSV 解析失败: %v", err)
	}
	if len(rows) != 251 {
		t.Fatalf("应导出 250 条 + 表头，得到 %d 行", len(rows))
	}
	// 首行带 UTF-8 BOM，方便 Excel 识别编码
	if !strings.HasSuffix(rows[0][0], "id") || rows[1][3] != "t,1" {
		t.Fatalf("CSV 内容不对: %v", rows[:2])
	}
}

// 客户端接入示例：完整流程（登录 → 心跳 → 刷新 → 公告/版本），可直接发给接入方。
// 放在这里而不是页面模板里，是因为三个示例加起来几百行，模板会被淹没。

export type ClientLang = 'csharp' | 'python' | 'cpp'

export const clientLangs = [
  { label: 'C#（Unity）', value: 'csharp' },
  { label: 'Python（联调）', value: 'python' },
  { label: 'C++（Android）', value: 'cpp' },
]

export function clientFileName(lang: ClientLang): string {
  if (lang === 'csharp') return 'DevourAuth.cs'
  if (lang === 'python') return 'devour_client.py'
  return 'devour_client.cpp'
}

const CSHARP = String.raw`// DevourAuth.cs —— 卡密验证客户端完整示例
// 依赖 Newtonsoft.Json（Unity: Package Manager 装 com.unity.nuget.newtonsoft-json）
// 用法：挂到一个空物体上，把 APP_CODE 换成对接信息里的应用 code，
//       调 Login("卡密")，成功后 SignedIn 变 true，Card 里是到期/余额/设备名额。
using System;
using System.Collections;
using System.Collections.Generic;
using System.Text;
using Newtonsoft.Json;
using Newtonsoft.Json.Linq;
using UnityEngine;
using UnityEngine.Networking;

public class DevourAuth : MonoBehaviour
{
    const string APP_CODE = "@@APP_CODE@@";
    const string BASE_URL = "@@BASE_URL@@";
    const string SAVE_KEY = "devour.auth";

    // 服务端返回的卡面：kind / expire_at / points / remain_times /
    // remaining_seconds / paused / device_limit / bound_devices / heartbeat_sec
    public JObject Card { get; private set; }
    public bool SignedIn { get; private set; }
    public string LastError { get; private set; }
    public string Notice { get; private set; }
    public JObject Latest { get; private set; }        // version / info / url / must
    public bool Maintenance { get; private set; }

    string access, refresh;
    long accessExp, refreshExp, heartbeatSec = 600;
    Coroutine heartbeat;

    [Serializable]
    class Saved
    {
        public string kami;
        public string access;
        public string refresh;
        public long access_exp;
        public long refresh_exp;
    }

    void Awake()
    {
        var saved = Load();
        if (saved != null && !string.IsNullOrEmpty(saved.kami))
            Login(saved.kami);              // 有卡密就自动登录；过期/封禁会走 LastError
        StartCoroutine(FetchMeta());
    }

    // ---------------- 对外接口 ----------------

    public void Login(string kami)
    {
        LastError = null;
        StartCoroutine(LoginFlow(kami.Trim()));
    }

    public void Logout()
    {
        if (!string.IsNullOrEmpty(access))
            StartCoroutine(Request("POST", "/v1/auth/logout", "{}", access, _ => { }));
        access = refresh = "";
        accessExp = refreshExp = 0;
        SignedIn = false;
        Card = null;
        Save("", "", "", 0, 0);
    }

    void OnApplicationQuit() => Save(CurrentKami, access, refresh, accessExp, refreshExp);

    // ---------------- 协议实现 ----------------

    IEnumerator LoginFlow(string kami)
    {
        var body = new JObject
        {
            ["app"] = APP_CODE,
            ["kami"] = kami,
            ["device"] = new JObject
            {
                ["fingerprint"] = DeviceFingerprint(),
                ["model"] = SystemInfo.deviceModel,
                ["brand"] = SystemInfo.deviceName,
                ["os"] = SystemInfo.operatingSystem,
                ["app_version"] = Application.version,
            },
            ["ts"] = UnixNow(),
            ["nonce"] = NewNonce(),
        };
        yield return Request("POST", "/v1/auth/login", body.ToString(Formatting.None), null, data =>
        {
            ApplyTokens(data);
            Card = data["card"] as JObject;
            heartbeatSec = Math.Max(60, (long?)Card?["heartbeat_sec"] ?? 600);
            SignedIn = true;
            Save(kami, access, refresh, accessExp, refreshExp);
            if (heartbeat != null) StopCoroutine(heartbeat);
            heartbeat = StartCoroutine(HeartbeatLoop());
        });
    }

    IEnumerator HeartbeatLoop()
    {
        while (SignedIn)
        {
            yield return new WaitForSecondsRealtime(heartbeatSec);
            if (!SignedIn) break;
            // access 只剩 3 分钟就先刷新，别等心跳 401
            if (accessExp > 0 && accessExp - UnixNow() < 180) yield return RefreshFlow();
            yield return SessionFlow();
            yield return FetchMeta();
        }
    }

    IEnumerator SessionFlow()
    {
        if (string.IsNullOrEmpty(access)) yield break;
        yield return Request("GET", "/v1/session", null, access, data =>
        {
            Card = data["card"] as JObject;
            heartbeatSec = Math.Max(60, (long?)Card?["heartbeat_sec"] ?? 600);
        }, code =>
        {
            // 卡没了/被回收/被封禁：直接下线，不要继续放行
            if (code == "CARD_EXPIRED" || code == "CARD_BANNED" || code == "CARD_RECYCLED" ||
                code == "CARD_EXHAUSTED" || code == "REVOKED" || code == "UNAUTHORIZED")
                ForceSignOut();
        });
    }

    IEnumerator RefreshFlow()
    {
        if (string.IsNullOrEmpty(refresh)) { ForceSignOut(); yield break; }
        var body = new JObject { ["refresh_token"] = refresh };
        yield return Request("POST", "/v1/auth/refresh", body.ToString(Formatting.None), null, ApplyTokens,
            _ => ForceSignOut());
    }

    IEnumerator FetchMeta()
    {
        string path = "/v1/meta?app=" + UnityWebRequest.EscapeURL(APP_CODE) +
                      "&fp=" + UnityWebRequest.EscapeURL(DeviceFingerprint());
        yield return Request("GET", path, null, null, data =>
        {
            Maintenance = (bool?)data["maintenance"] ?? false;
            var sb = new StringBuilder();
            if (data["notices"] is JArray notices)
                foreach (JObject n in notices)
                {
                    string title = (string)n["title"], text = (string)n["body"];
                    if (!string.IsNullOrEmpty(title)) sb.Append(title).Append('\n');
                    if (!string.IsNullOrEmpty(text)) sb.Append(text).Append('\n');
                }
            Notice = sb.ToString().Trim();
            Latest = data["release"] as JObject;     // 没有新版本时这个字段不存在
        });
    }

    void ApplyTokens(JObject data)
    {
        access = (string)data["access_token"];
        refresh = (string)data["refresh_token"];
        accessExp = (long?)data["access_exp"] ?? 0;
        refreshExp = (long?)data["refresh_exp"] ?? 0;
        Card = data["card"] as JObject ?? Card;
    }

    void ForceSignOut()
    {
        SignedIn = false;
        access = refresh = "";
        accessExp = refreshExp = 0;
        Card = null;
        Save(CurrentKami, "", "", 0, 0);
    }

    // ---------------- HTTP ----------------

    IEnumerator Request(string method, string path, string body, string bearer,
                       Action<JObject> onOk, Action<string> onFail = null)
    {
        using (var req = new UnityWebRequest(BASE_URL + path, method))
        {
            if (body != null)
            {
                byte[] raw = Encoding.UTF8.GetBytes(body);
                req.uploadHandler = new UploadHandlerRaw(raw);
                req.SetRequestHeader("Content-Type", "application/json");
            }
            if (!string.IsNullOrEmpty(bearer))
                req.SetRequestHeader("Authorization", "Bearer " + bearer);
            req.downloadHandler = new DownloadHandlerBuffer();
            req.timeout = 15;
            yield return req.SendWebRequest();

            string text = req.downloadHandler.text;
            if (req.result != UnityWebRequest.Result.Success && string.IsNullOrEmpty(text))
            {
                LastError = "网络异常：" + req.error;
                yield break;
            }
            JObject root;
            try { root = JObject.Parse(text); }
            catch (Exception)
            {
                LastError = "服务端返回无法解析";
                yield break;
            }
            if ((bool?)root["ok"] == true)
            {
                LastError = null;
                onOk(root["data"] as JObject ?? new JObject());
            }
            else
            {
                string code = (string)root["code"] ?? "";
                LastError = MessageOf(code, (string)root["message"]);
                onFail?.Invoke(code);
            }
        }
    }

    public static string MessageOf(string code, string serverMessage)
    {
        switch (code)
        {
            case "CARD_NOT_FOUND": return "卡密不存在，请检查输入";
            case "CARD_EXPIRED": return "卡密已到期";
            case "CARD_BANNED": return "卡密已被封禁";
            case "CARD_RECYCLED": return "卡密已回收";
            case "CARD_EXHAUSTED": return "卡密次数或点数已用完";
            case "DEVICE_LIMIT": return "设备数量已满，请先解绑旧设备";
            case "DEVICE_MISMATCH": return "设备不匹配，请联系客服";
            case "UNAUTHORIZED": return "登录已过期，请重新登录";
            case "REVOKED": return "登录已失效，请重新登录";
            case "PAUSED": return "卡密已暂停";
            case "RATE_LIMITED": return "操作太频繁，请稍后再试";
            case "MAINTENANCE": return "服务器维护中，请稍后再试";
            case "BAD_INPUT": return "请求有误，请检查设备时间";
            case "INTERNAL": return "服务端异常，请稍后再试";
            default: return string.IsNullOrEmpty(serverMessage) ? "验证失败，请稍后再试" : serverMessage;
        }
    }

    // ---------------- 杂项 ----------------

    // 设备指纹：同一台设备清数据重装不变，服务端靠它计设备名额。
    public static string DeviceFingerprint()
    {
        string key = "devour.device_id";
        string saved = PlayerPrefs.GetString(key, "");
        if (!string.IsNullOrEmpty(saved)) return saved;
        string fp;
#if UNITY_ANDROID && !UNITY_EDITOR
        try
        {
            var player = new AndroidJavaClass("com.unity3d.player.UnityPlayer");
            var activity = player.GetStatic<AndroidJavaObject>("currentActivity");
            var resolver = activity.Call<AndroidJavaObject>("getContentResolver");
            var secure = new AndroidJavaClass("android.provider.Settings$Secure");
            fp = secure.CallStatic<string>("getString", resolver, "android_id");
        }
        catch (Exception) { fp = SystemInfo.deviceUniqueIdentifier; }
#else
        fp = SystemInfo.deviceUniqueIdentifier;
#endif
        if (string.IsNullOrEmpty(fp)) fp = Guid.NewGuid().ToString("N");
        fp = fp.ToUpperInvariant();
        PlayerPrefs.SetString(key, fp);
        PlayerPrefs.Save();
        return fp;
    }

    static long UnixNow() => DateTimeOffset.UtcNow.ToUnixTimeSeconds();

    static string NewNonce() => Guid.NewGuid().ToString("N");

    string CurrentKami
    {
        get { var s = Load(); return s?.kami ?? ""; }
    }

    Saved Load()
    {
        string raw = PlayerPrefs.GetString(SAVE_KEY, "");
        if (string.IsNullOrEmpty(raw)) return null;
        try { return JsonConvert.DeserializeObject<Saved>(raw); }
        catch (Exception) { return null; }
    }

    void Save(string kami, string a, string r, long aexp, long rexp)
    {
        var s = new Saved { kami = kami, access = a, refresh = r, access_exp = aexp, refresh_exp = rexp };
        PlayerPrefs.SetString(SAVE_KEY, JsonConvert.SerializeObject(s));
        PlayerPrefs.Save();
    }
}
`

const PYTHON = String.raw`# devour_client.py —— 卡密验证客户端完整示例（联调/自动化用）
# 依赖：pip install requests
# 跑法：python3 devour_client.py SCFS-XXXX-XXXX
# 状态存到同目录 devour_auth.json，下次启动 is 自动用 refresh 续上。
import hashlib
import json
import os
import secrets
import sys
import threading
import time

import requests

BASE_URL = "@@BASE_URL@@"
APP_CODE = "@@APP_CODE@@"
STATE_FILE = os.path.join(os.path.dirname(os.path.abspath(__file__)), "devour_auth.json")

MESSAGES = {
    "CARD_NOT_FOUND": "卡密不存在，请检查输入",
    "CARD_EXPIRED": "卡密已到期",
    "CARD_BANNED": "卡密已被封禁",
    "CARD_RECYCLED": "卡密已回收",
    "CARD_EXHAUSTED": "卡密次数或点数已用完",
    "DEVICE_LIMIT": "设备数量已满，请先解绑旧设备",
    "DEVICE_MISMATCH": "设备不匹配，请联系客服",
    "UNAUTHORIZED": "登录已过期，请重新登录",
    "REVOKED": "登录已失效，请重新登录",
    "PAUSED": "卡密已暂停",
    "RATE_LIMITED": "操作太频繁，请稍后再试",
    "MAINTENANCE": "服务器维护中，请稍后再试",
    "BAD_INPUT": "请求有误，请检查设备时间",
    "INTERNAL": "服务端异常，请稍后再试",
}
DEAD_CODES = {"CARD_EXPIRED", "CARD_BANNED", "CARD_RECYCLED",
              "CARD_EXHAUSTED", "REVOKED", "UNAUTHORIZED"}


class AuthError(Exception):
    def __init__(self, code, message):
        super().__init__(MESSAGES.get(code, message or "验证失败，请稍后再试"))
        self.code = code


def device_fingerprint():
    """同一台设备保持不变的标识：Android 用 Settings.Secure.ANDROID_ID，
    桌面端用 machine-id。拿不到就用文件里存的一次性 UUID。"""
    path = os.path.join(os.path.dirname(STATE_FILE), "devour_device.txt")
    if os.path.exists(path):
        with open(path, encoding="utf-8") as f:
            value = f.read().strip()
        if value:
            return value
    value = ""
    if sys.platform.startswith("android"):
        try:
            from jnius import autoclass  # python-for-android / pyjnius
            Context = autoclass("android.content.Context")
            Secure = autoclass("android.provider.Settings$Secure")
            activity = autoclass("org.kivy.android.PythonActivity").mActivity
            value = Secure.getString(activity.getContentResolver(), "android_id")
        except Exception:
            value = ""
    if not value:
        for candidate in ("/etc/machine-id", "/var/lib/dbus/machine-id"):
            try:
                with open(candidate, encoding="utf-8") as f:
                    value = f.read().strip()
                if value:
                    break
            except OSError:
                continue
    if not value:
        value = secrets.token_hex(8)
    value = value.upper()
    with open(path, "w", encoding="utf-8") as f:
        f.write(value)
    return value


class Client:
    def __init__(self):
        self.session = requests.Session()
        self.state = self._load()
        self.card = None
        self.heartbeat_sec = 600

    # ---- 状态持久化 ----
    def _load(self):
        try:
            with open(STATE_FILE, encoding="utf-8") as f:
                return json.load(f)
        except (OSError, ValueError):
            return {}

    def _save(self):
        with open(STATE_FILE, "w", encoding="utf-8") as f:
            json.dump(self.state, f, ensure_ascii=False, indent=2)

    # ---- HTTP ----
    def _call(self, method, path, body=None, bearer=None):
        headers = {"Content-Type": "application/json"}
        if bearer:
            headers["Authorization"] = "Bearer " + bearer
        r = self.session.request(method, BASE_URL + path, data=body,
                                 headers=headers, timeout=15)
        try:
            root = r.json()
        except ValueError:
            raise AuthError("INTERNAL", "服务端返回无法解析")
        if not root.get("ok"):
            raise AuthError(root.get("code", ""), root.get("message", ""))
        return root.get("data") or {}

    # ---- 协议 ----
    def login(self, kami):
        body = json.dumps({
            "app": APP_CODE,
            "kami": kami.strip(),
            "device": {
                "fingerprint": device_fingerprint(),
                "model": "python-client",
                "brand": "devour",
                "os": "python " + sys.version.split()[0],
                "app_version": "1.0.0",
            },
            "ts": int(time.time()),
            "nonce": secrets.token_hex(16),
        })
        data = self._call("POST", "/v1/auth/login", body)
        self._apply(data, kami)

    def refresh(self):
        token = self.state.get("refresh")
        if not token:
            raise AuthError("UNAUTHORIZED", "没有 refresh_token")
        data = self._call("POST", "/v1/auth/refresh",
                          json.dumps({"refresh_token": token}))
        self._apply(data, self.state.get("kami", ""))

    def session(self):
        data = self._call("GET", "/v1/session", bearer=self.state.get("access"))
        self._apply({"card": data.get("card")}, self.state.get("kami", ""))

    def meta(self):
        return self._call("GET", "/v1/meta?app=%s&fp=%s" % (APP_CODE, device_fingerprint()))

    def logout(self):
        try:
            self._call("POST", "/v1/auth/logout", "{}", self.state.get("access"))
        except AuthError:
            pass
        self.state = {}
        self._save()

    def _apply(self, data, kami):
        for key in ("access_token", "access_exp", "refresh_token", "refresh_exp"):
            if data.get(key):
                self.state[key] = data[key]
        if kami:
            self.state["kami"] = kami
        card = data.get("card")
        if card:
            self.card = card
            self.heartbeat_sec = max(60, int(card.get("heartbeat_sec") or 600))
        self._save()

    # ---- 心跳 ----
    def run_heartbeat(self, stop_event):
        while not stop_event.wait(self.heartbeat_sec):
            try:
                if self.state.get("access_exp", 0) - time.time() < 180:
                    self.refresh()
                self.session()
                print("[心跳] 在线，到期", self.card.get("expire_at"))
            except AuthError as e:
                if e.code in DEAD_CODES:
                    print("[下线]", e)
                    self.state = {}
                    self._save()
                    return
                print("[心跳] 失败（稍后重试）：", e)


if __name__ == "__main__":
    client = Client()
    if len(sys.argv) > 1:
        client.login(sys.argv[1])
    elif client.state.get("refresh"):
        client.refresh()
    else:
        sys.exit("用法: python3 devour_client.py <卡密>")

    print("已登录，卡面：", json.dumps(client.card, ensure_ascii=False))
    print("公告与版本：", json.dumps(client.meta(), ensure_ascii=False))
    stop = threading.Event()
    threading.Thread(target=client.run_heartbeat, args=(stop,), daemon=True).start()
    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        stop.set()
`

const CPP = String.raw`// devour_client.cpp —— Android NDK 卡密客户端完整示例
// 依赖：nlohmann/json（单头文件）；HTTPS 走 Java 的 HttpsURLConnection（JNI），
//       不引第三方 TLS 库，和 Devour 客户端同一套做法。
// 编译（Android.mk 里加 LOCAL_SRC_FILES 即可）：
//   clang++ -std=c++17 -fPIC -shared devour_client.cpp -llog -o libdevour_client.so
// 用法：在 JNI_OnLoad 里调 devour::client::Start("卡密", "APP_CODE");
#include <jni.h>
#include <unistd.h>

#include <atomic>
#include <chrono>
#include <cstdio>
#include <mutex>
#include <string>
#include <thread>

#include "nlohmann/json.hpp"

namespace devour {
namespace client {

using json = nlohmann::json;

constexpr const char* kBaseUrl = "@@BASE_URL@@";
constexpr const char* kAppCode = "@@APP_CODE@@";

struct State {
  std::string access;
  std::string refresh;
  long long access_exp = 0;
  long long refresh_exp = 0;
  long long heartbeat_sec = 600;
  json card;
  bool signed_in = false;
  std::string message;
};

std::mutex g_mutex;
State g_state;
JavaVM* g_vm = nullptr;
std::atomic<bool> g_started{false};

long long Now() { return static_cast<long long>(::time(nullptr)); }

std::string NewNonce() {
  unsigned char raw[16] = {};
  FILE* fp = fopen("/dev/urandom", "rb");
  if (fp != nullptr) {
    fread(raw, 1, sizeof(raw), fp);
    fclose(fp);
  }
  static const char* hex = "0123456789abcdef";
  std::string out;
  for (unsigned char c : raw) {
    out.push_back(hex[c >> 4]);
    out.push_back(hex[c & 0xF]);
  }
  return out;
}

// ---------------- JNI HTTP ----------------

struct Env {
  JNIEnv* env = nullptr;
  bool attached = false;
  ~Env() {
    if (attached && g_vm != nullptr) g_vm->DetachCurrentThread();
  }
};

Env GetEnv() {
  Env out;
  if (g_vm == nullptr) return out;
  jint code = g_vm->GetEnv(reinterpret_cast<void**>(&out.env), JNI_VERSION_1_6);
  if (code == JNI_EDETACHED) {
    if (g_vm->AttachCurrentThread(&out.env, nullptr) == JNI_OK) out.attached = true;
    else out.env = nullptr;
  } else if (code != JNI_OK) {
    out.env = nullptr;
  }
  return out;
}

std::string CallString(JNIEnv* env, jobject obj, jmethodID id) {
  auto value = static_cast<jstring>(env->CallObjectMethod(obj, id));
  if (value == nullptr) return {};
  const char* chars = env->GetStringUTFChars(value, nullptr);
  std::string out = chars == nullptr ? "" : chars;
  if (chars != nullptr) env->ReleaseStringUTFChars(value, chars);
  env->DeleteLocalRef(value);
  return out;
}

struct Response {
  int status = 0;   // 0 = 网络失败
  std::string body;
  std::string error;
};

Response Request(const std::string& method, const std::string& path,
                 const std::string& body, const std::string& bearer) {
  Response out;
  Env scope = GetEnv();
  JNIEnv* env = scope.env;
  if (env == nullptr) {
    out.error = "no jni env";
    return out;
  }
  // 一次请求里的局部引用都交给这个 frame，循环心跳也不会把引用槽占满。
  if (env->PushLocalFrame(64) != JNI_OK) {
    out.error = "no jni frame";
    return out;
  }
  auto text = [env](const std::string& value) {
    return env->NewStringUTF(value.c_str());
  };

  jclass url_cls = env->FindClass("java/net/URL");
  jclass conn_cls = env->FindClass("java/net/HttpURLConnection");
  jclass os_cls = env->FindClass("java/io/OutputStream");
  jclass is_cls = env->FindClass("java/io/InputStream");
  jclass isr_cls = env->FindClass("java/io/InputStreamReader");
  jclass reader_cls = env->FindClass("java/io/BufferedReader");
  if (url_cls == nullptr || conn_cls == nullptr || os_cls == nullptr ||
      is_cls == nullptr || isr_cls == nullptr || reader_cls == nullptr) {
    env->ExceptionClear();
    env->PopLocalFrame(nullptr);
    out.error = "no http classes";
    return out;
  }

  const jmethodID url_ctor =
      env->GetMethodID(url_cls, "<init>", "(Ljava/lang/String;)V");
  const jmethodID open =
      env->GetMethodID(url_cls, "openConnection", "()Ljava/net/URLConnection;");
  const jmethodID set_method =
      env->GetMethodID(conn_cls, "setRequestMethod", "(Ljava/lang/String;)V");
  const jmethodID set_prop = env->GetMethodID(
      conn_cls, "setRequestProperty", "(Ljava/lang/String;Ljava/lang/String;)V");
  const jmethodID set_timeout =
      env->GetMethodID(conn_cls, "setConnectTimeout", "(I)V");
  const jmethodID set_read =
      env->GetMethodID(conn_cls, "setReadTimeout", "(I)V");
  const jmethodID set_do_output =
      env->GetMethodID(conn_cls, "setDoOutput", "(Z)V");
  const jmethodID get_output = env->GetMethodID(
      conn_cls, "getOutputStream", "()Ljava/io/OutputStream;");
  const jmethodID get_input = env->GetMethodID(
      conn_cls, "getInputStream", "()Ljava/io/InputStream;");
  const jmethodID get_error = env->GetMethodID(
      conn_cls, "getErrorStream", "()Ljava/io/InputStream;");
  const jmethodID get_code =
      env->GetMethodID(conn_cls, "getResponseCode", "()I");
  const jmethodID disconnect =
      env->GetMethodID(conn_cls, "disconnect", "()V");
  const jmethodID write = env->GetMethodID(os_cls, "write", "([B)V");

  jobject url = env->NewObject(url_cls, url_ctor, text(std::string(kBaseUrl) + path));
  jobject conn = url == nullptr ? nullptr : env->CallObjectMethod(url, open);
  if (conn == nullptr || env->ExceptionCheck()) {
    env->ExceptionClear();
    env->PopLocalFrame(nullptr);
    out.error = "connect failed";
    return out;
  }

  env->CallVoidMethod(conn, set_timeout, 8000);
  env->CallVoidMethod(conn, set_read, 12000);
  env->CallVoidMethod(conn, set_method, text(method));
  env->CallVoidMethod(conn, set_prop, text("Content-Type"),
                      text("application/json"));
  if (!bearer.empty()) {
    env->CallVoidMethod(conn, set_prop, text("Authorization"),
                        text("Bearer " + bearer));
  }
  if (!body.empty()) {
    env->CallVoidMethod(conn, set_do_output, JNI_TRUE);
    jobject output = env->CallObjectMethod(conn, get_output);
    if (output != nullptr) {
      jbyteArray bytes = env->NewByteArray(static_cast<jsize>(body.size()));
      env->SetByteArrayRegion(bytes, 0, static_cast<jsize>(body.size()),
                              reinterpret_cast<const jbyte*>(body.data()));
      env->CallVoidMethod(output, write, bytes);
    }
  }

  out.status = env->CallIntMethod(conn, get_code);
  // 4xx/5xx 时 getInputStream 会抛，错误体要从 getErrorStream 拿。
  jobject input = env->CallObjectMethod(conn, get_input);
  if (input == nullptr || env->ExceptionCheck()) {
    env->ExceptionClear();
    input = env->CallObjectMethod(conn, get_error);
  }
  if (input != nullptr && !env->ExceptionCheck()) {
    const jmethodID isr_ctor = env->GetMethodID(
        isr_cls, "<init>", "(Ljava/io/InputStream;)V");
    const jmethodID reader_ctor =
        env->GetMethodID(reader_cls, "<init>", "(Ljava/io/Reader;)V");
    const jmethodID read_line = env->GetMethodID(
        reader_cls, "readLine", "()Ljava/lang/String;");
    jobject reader =
        env->NewObject(reader_cls, reader_ctor,
                       env->NewObject(isr_cls, isr_ctor, input));
    if (reader != nullptr) {
      for (;;) {
        auto line = static_cast<jstring>(env->CallObjectMethod(reader, read_line));
        if (line == nullptr || env->ExceptionCheck()) break;
        const char* chars = env->GetStringUTFChars(line, nullptr);
        if (chars != nullptr) {
          if (!out.body.empty()) out.body.push_back('\n');
          out.body += chars;
          env->ReleaseStringUTFChars(line, chars);
        }
      }
      env->ExceptionClear();
    }
  }
  env->CallVoidMethod(conn, disconnect);
  env->PopLocalFrame(nullptr);
  return out;
}

// ---------------- 协议 ----------------

const char* MessageOf(const std::string& code) {
  if (code == "CARD_NOT_FOUND") return "卡密不存在，请检查输入";
  if (code == "CARD_EXPIRED") return "卡密已到期";
  if (code == "CARD_BANNED") return "卡密已被封禁";
  if (code == "CARD_RECYCLED") return "卡密已回收";
  if (code == "CARD_EXHAUSTED") return "卡密次数或点数已用完";
  if (code == "DEVICE_LIMIT") return "设备数量已满，请先解绑旧设备";
  if (code == "DEVICE_MISMATCH") return "设备不匹配，请联系客服";
  if (code == "UNAUTHORIZED") return "登录已过期，请重新登录";
  if (code == "REVOKED") return "登录已失效，请重新登录";
  if (code == "PAUSED") return "卡密已暂停";
  if (code == "RATE_LIMITED") return "操作太频繁，请稍后再试";
  if (code == "MAINTENANCE") return "服务器维护中，请稍后再试";
  if (code == "BAD_INPUT") return "请求有误，请检查设备时间";
  if (code == "INTERNAL") return "服务端异常，请稍后再试";
  return "验证失败，请稍后再试";
}

bool IsDead(const std::string& code) {
  return code == "CARD_EXPIRED" || code == "CARD_BANNED" ||
         code == "CARD_RECYCLED" || code == "CARD_EXHAUSTED" ||
         code == "REVOKED" || code == "UNAUTHORIZED";
}

void SignOut(const std::string& message) {
  std::lock_guard<std::mutex> lock(g_mutex);
  g_state.signed_in = false;
  g_state.access.clear();
  g_state.refresh.clear();
  g_state.access_exp = 0;
  g_state.refresh_exp = 0;
  g_state.card = json::object();
  g_state.message = message;
}

// 返回 true 表示拿到了结论；error 里是错误码（成功时为空）。
bool Call(const std::string& method, const std::string& path,
          const std::string& body, const std::string& bearer, json* data,
          std::string* code) {
  const Response res = Request(method, path, body, bearer);
  if (res.status == 0) return false;
  json root;
  try {
    root = json::parse(res.body);
  } catch (const std::exception&) {
    *code = "INTERNAL";
    return true;
  }
  if (root.value("ok", false)) {
    if (data != nullptr) *data = root.contains("data") ? root["data"] : json::object();
    code->clear();
    return true;
  }
  *code = root.value("code", std::string("INTERNAL"));
  *data = json::object();
  return true;
}

void Apply(const json& data) {
  std::lock_guard<std::mutex> lock(g_mutex);
  if (data.contains("access_token")) g_state.access = data["access_token"];
  if (data.contains("refresh_token")) g_state.refresh = data["refresh_token"];
  if (data.contains("access_exp")) g_state.access_exp = data["access_exp"];
  if (data.contains("refresh_exp")) g_state.refresh_exp = data["refresh_exp"];
  if (data.contains("card")) {
    g_state.card = data["card"];
    const long long sec = g_state.card.value("heartbeat_sec", 600LL);
    g_state.heartbeat_sec = sec < 60 ? 60 : sec;
  }
  g_state.signed_in = true;
  g_state.message.clear();
}

bool Login(const std::string& kami, const std::string& fingerprint) {
  json body = {
      {"app", kAppCode},
      {"kami", kami},
      {"device",
       {{"fingerprint", fingerprint},
        {"model", "android"},
        {"brand", "devour"},
        {"os", "android"},
        {"app_version", "1.0.0"}}},
      {"ts", Now()},
      {"nonce", NewNonce()},
  };
  json data;
  std::string code;
  if (!Call("POST", "/v1/auth/login", body.dump(), "", &data, &code)) {
    SignOut("网络异常，请稍后再试");
    return false;
  }
  if (!code.empty()) {
    SignOut(MessageOf(code));
    return false;
  }
  Apply(data);
  return true;
}

bool Refresh() {
  std::string token;
  {
    std::lock_guard<std::mutex> lock(g_mutex);
    token = g_state.refresh;
  }
  if (token.empty()) {
    SignOut("登录已过期，请重新登录");
    return false;
  }
  json data;
  std::string code;
  if (!Call("POST", "/v1/auth/refresh", json{{"refresh_token", token}}.dump(),
            "", &data, &code)) {
    return false;
  }
  if (!code.empty()) {
    SignOut(MessageOf(code));
    return false;
  }
  Apply(data);
  return true;
}

bool Session() {
  std::string token;
  {
    std::lock_guard<std::mutex> lock(g_mutex);
    if (!g_state.signed_in) return false;
    token = g_state.access;
  }
  json data;
  std::string code;
  if (!Call("GET", "/v1/session", "", token, &data, &code)) return false;
  if (!code.empty()) {
    if (IsDead(code)) SignOut(MessageOf(code));
    return false;
  }
  Apply(data);
  return true;
}

void Heartbeat() {
  for (;;) {
    long long interval = 600;
    {
      std::lock_guard<std::mutex> lock(g_mutex);
      interval = g_state.heartbeat_sec;
    }
    std::this_thread::sleep_for(std::chrono::seconds(interval));
    std::string access;
    long long access_exp = 0;
    {
      std::lock_guard<std::mutex> lock(g_mutex);
      if (!g_state.signed_in) continue;
      access = g_state.access;
      access_exp = g_state.access_exp;
    }
    // access 只剩 3 分钟先刷新，否则心跳会白跑一次 401
    if (access_exp > 0 && access_exp - Now() < 180) {
      Refresh();
      continue;
    }
    Session();
  }
}

// 在 JNI_OnLoad（或任意 Java 线程）里调一次；fingerprint 用 ANDROID_ID。
void Start(const std::string& kami, const std::string& fingerprint) {
  if (g_started.exchange(true)) return;
  std::thread([kami, fingerprint] {
    Login(kami, fingerprint);
    Heartbeat();
  }).detach();
}

State Snapshot() {
  std::lock_guard<std::mutex> lock(g_mutex);
  return g_state;
}

}  // namespace client
}  // namespace devour

// 取 ANDROID_ID：清数据重装不变，服务端靠它计设备名额。
extern "C" JNIEXPORT jstring JNICALL
Java_com_example_devour_Client_fingerprint(JNIEnv* env, jclass) {
  jclass activity_thread = env->FindClass("android/app/ActivityThread");
  jmethodID current = env->GetStaticMethodID(
      activity_thread, "currentApplication", "()Landroid/app/Application;");
  jobject app = env->CallStaticObjectMethod(activity_thread, current);
  jclass context_cls = env->FindClass("android/content/Context");
  jmethodID get_resolver = env->GetMethodID(
      context_cls, "getContentResolver", "()Landroid/content/ContentResolver;");
  jobject resolver = env->CallObjectMethod(app, get_resolver);
  jclass secure = env->FindClass("android/provider/Settings$Secure");
  jmethodID get_string = env->GetStaticMethodID(
      secure, "getString",
      "(Landroid/content/ContentResolver;Ljava/lang/String;)Ljava/lang/String;");
  jstring key = env->NewStringUTF("android_id");
  auto value = static_cast<jstring>(
      env->CallStaticObjectMethod(secure, get_string, resolver, key));
  return value;
}

extern "C" JNIEXPORT void JNICALL
Java_com_example_devour_Client_start(JNIEnv* env, jclass, jstring jkami,
                                    jstring jfingerprint) {
  const char* kami = env->GetStringUTFChars(jkami, nullptr);
  const char* fp = env->GetStringUTFChars(jfingerprint, nullptr);
  devour::client::Start(kami, fp);
  env->ReleaseStringUTFChars(jkami, kami);
  env->ReleaseStringUTFChars(jfingerprint, fp);
}
`

export function clientSample(lang: ClientLang, appCode: string, baseUrl: string): string {
  const source = lang === 'csharp' ? CSHARP : lang === 'python' ? PYTHON : CPP
  return source
    .replaceAll('@@APP_CODE@@', appCode || 'YOUR_APP_CODE')
    .replaceAll('@@BASE_URL@@', baseUrl || 'https://net.namikao.com')
}

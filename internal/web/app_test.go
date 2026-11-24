package web

import (
	"database/sql"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"testing"
	"time"

	"cafego/internal/store"
	"golang.org/x/crypto/bcrypt"
)

type browser struct {
	t          *testing.T
	client     *http.Client
	base, csrf string
}

var csrfPattern = regexp.MustCompile(`name="csrf" value="([a-f0-9]{64})"`)

// handlerServer exercises the complete HTTP stack without requiring sockets.
// CookieJar, redirects, parsing, middleware and SQLite are all real.
type handlerServer struct {
	URL     string
	Handler http.Handler
}
type handlerTransport struct{ handler http.Handler }

func (tr handlerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	request := r.Clone(r.Context())
	request.RequestURI = request.URL.RequestURI()
	tr.handler.ServeHTTP(recorder, request)
	response := recorder.Result()
	response.Request = r
	return response, nil
}

func setup(t *testing.T) (*sql.DB, *handlerServer) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err = store.Seed(db); err != nil {
		t.Fatal(err)
	}
	server := &handlerServer{URL: "http://cafego.test", Handler: New(db, false).Handler()}
	t.Cleanup(func() { db.Close() })
	return db, server
}
func newBrowser(t *testing.T, s *handlerServer) *browser {
	t.Helper()
	jar, _ := cookiejar.New(nil)
	return &browser{t: t, base: s.URL, client: &http.Client{Transport: handlerTransport{handler: s.Handler}, Jar: jar, Timeout: 5 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
}
func (b *browser) get(path string) string {
	b.t.Helper()
	for n := 0; n < 4; n++ {
		res, err := b.client.Get(b.base + path)
		if err != nil {
			b.t.Fatal(err)
		}
		raw, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			b.t.Fatal(err)
		}
		if res.StatusCode == 303 {
			path = res.Header.Get("Location")
			continue
		}
		if res.StatusCode != 200 {
			b.t.Fatalf("GET %s: %d %s", path, res.StatusCode, raw)
		}
		body := string(raw)
		if m := csrfPattern.FindStringSubmatch(body); len(m) > 0 {
			b.csrf = m[1]
		}
		return body
	}
	b.t.Fatal("redirect loop")
	return ""
}
func (b *browser) post(path string, values url.Values, want int) string {
	b.t.Helper()
	if values == nil {
		values = url.Values{}
	}
	values.Set("csrf", b.csrf)
	return b.rawPost(path, values, want)
}
func (b *browser) rawPost(path string, values url.Values, want int) string {
	b.t.Helper()
	res, err := b.client.PostForm(b.base+path, values)
	if err != nil {
		b.t.Fatal(err)
	}
	raw, err := io.ReadAll(res.Body)
	res.Body.Close()
	if err != nil {
		b.t.Fatal(err)
	}
	if res.StatusCode != want {
		b.t.Fatalf("POST %s: got %d want %d: %s", path, res.StatusCode, want, raw)
	}
	return string(raw)
}
func (b *browser) register(login string) {
	b.get("/register")
	b.post("/register", url.Values{"login": {login}, "password": {"a-safe-test-password"}}, 303)
	b.get("/")
}
func (b *browser) login(login string) {
	b.get("/login")
	b.post("/login", url.Values{"login": {login}, "password": {"a-safe-test-password"}}, 303)
	b.get("/")
}
func count(t *testing.T, db *sql.DB, q string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
func insertAdmin(t *testing.T, db *sql.DB) {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("a-safe-test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO users(login,password_hash,role) VALUES('admin',?,'admin')", string(hash)); err != nil {
		t.Fatal(err)
	}
}

func TestAuthenticationAndSecurity(t *testing.T) {
	db, s := setup(t)
	b := newBrowser(t, s)
	b.get("/register")
	old := b.csrf
	b.rawPost("/register", url.Values{"login": {"alice"}, "password": {"a-safe-test-password"}}, 403)
	b.post("/register", url.Values{"login": {"x"}, "password": {"short"}}, 200)
	if count(t, db, "SELECT count(*) FROM users") != 0 {
		t.Fatal("invalid registration stored")
	}
	b.register("alice")
	if old == b.csrf {
		t.Fatal("CSRF did not rotate")
	}
	var hash, role string
	if err := db.QueryRow("SELECT password_hash,role FROM users WHERE login='alice'").Scan(&hash, &role); err != nil {
		t.Fatal(err)
	}
	if role != "client" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("a-safe-test-password")) != nil {
		t.Fatal("role/hash invalid")
	}
	for _, path := range []string{"/admin"} {
		res, err := b.client.Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("unguarded %s", path)
		}
	}
	outsider := newBrowser(t, s)
	outsider.get("/login")
	outsider.rawPost("/login", url.Values{"csrf": {b.csrf}, "login": {"alice"}, "password": {"a-safe-test-password"}}, 403)
	b.post("/logout", nil, 303)
	res, err := b.client.Get(s.URL + "/admin")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 303 {
		t.Fatal("logout left access")
	}
	b.get("/login")
	b.post("/login", url.Values{"login": {"alice"}, "password": {"wrong-password"}}, 200)
	b.login("alice")
	b.get("/")
	b.post("/register", url.Values{"login": {"ALICE"}, "password": {"a-safe-test-password"}}, 200)
	if count(t, db, "SELECT count(*) FROM users") != 1 {
		t.Fatal("case-insensitive duplicate")
	}
	u, _ := url.Parse(s.URL)
	for _, c := range b.client.Jar.Cookies(u) {
		if c.Name == "cafego_session" && len(c.Value) != 64 {
			t.Fatal("weak token")
		}
	}
	// Check flags directly: cookie jars intentionally omit those flags on reads.
	fresh := newBrowser(t, s)
	response, err := fresh.client.Get(s.URL + "/login")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	cookies := response.Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("cookie flags missing")
	}
	if response.Header.Get("Content-Security-Policy") == "" {
		t.Fatal("missing CSP")
	}
}

func TestAdminLoginAfterDatabaseReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "persist.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	insertAdmin(t, db)
	s := &handlerServer{URL: "http://cafego.test", Handler: New(db, false).Handler()}
	b := newBrowser(t, s)
	b.login("admin")
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	b.client.Transport = handlerTransport{handler: New(reopened, false).Handler()}
	b.get("/admin") // The existing session survives the restart too.
	b.post("/logout", nil, 303)
	b.login("admin") // The original password still works after a fresh login.
	b.get("/admin")
}

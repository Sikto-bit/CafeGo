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
	"strconv"
	"strings"
	"sync"
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
	for _, path := range []string{"/admin", "/admin/categories", "/admin/dishes", "/admin/orders"} {
		res, err := b.client.Get(s.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 403 {
			t.Fatalf("unguarded %s", path)
		}
	}
	b.post("/admin/categories/save", url.Values{"name": {"Injected"}}, 403)
	b.post("/admin/categories/delete", url.Values{"id": {"1"}}, 403)
	b.post("/admin/dishes/save", url.Values{"name": {"Injected"}}, 403)
	b.post("/admin/dishes/delete", url.Values{"id": {"1"}}, 403)
	b.post("/admin/orders/status", url.Values{"id": {"1"}, "status": {"ready"}}, 403)
	outsider := newBrowser(t, s)
	outsider.get("/login")
	outsider.rawPost("/login", url.Values{"csrf": {b.csrf}, "login": {"alice"}, "password": {"a-safe-test-password"}}, 403)
	b.post("/logout", nil, 303)
	res, err := b.client.Get(s.URL + "/cart")
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
	b.get("/cart")
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

func TestCRUDCartAndOrders(t *testing.T) {
	db, s := setup(t)
	insertAdmin(t, db)
	a := newBrowser(t, s)
	a.login("admin")
	for _, path := range []string{"/admin", "/admin/categories", "/admin/dishes", "/admin/orders", "/cart", "/orders"} {
		a.get(path)
	}
	a.post("/admin/categories/save", url.Values{"name": {"Завтраки"}}, 303)
	cat := count(t, db, "SELECT id FROM categories WHERE name='Завтраки'")
	a.get("/admin/categories?edit=" + strconv.Itoa(cat))
	a.post("/admin/categories/save", url.Values{"id": {strconv.Itoa(cat)}, "name": {"Завтраки дня"}}, 303)
	a.post("/admin/categories/save", url.Values{"name": {"Завтраки дня"}}, 200)
	form := url.Values{"category_id": {strconv.Itoa(cat)}, "name": {"<script>alert(1)</script>"}, "description": {"Овсянка с ягодами"}, "price": {"123,45"}}
	a.post("/admin/dishes/save", form, 303)
	dish := count(t, db, "SELECT id FROM dishes WHERE description='Овсянка с ягодами'")
	id := strconv.Itoa(dish)
	a.get("/admin/dishes?edit=" + id)
	a.post("/admin/categories/delete", url.Values{"id": {strconv.Itoa(cat)}}, 200)
	if count(t, db, "SELECT count(*) FROM categories WHERE id=?", cat) != 1 {
		t.Fatal("deleted occupied category")
	}
	b := newBrowser(t, s)
	b.register("client1")
	body := b.get("/?category=" + strconv.Itoa(cat))
	if strings.Contains(body, "<script>") || !strings.Contains(body, "&lt;script&gt;") {
		t.Fatal("HTML not escaped")
	}
	b.post("/cart/add", url.Values{"dish_id": {id}}, 303)
	b.post("/cart/update", url.Values{"dish_id": {id}, "quantity": {"0"}}, 400)
	b.post("/cart/update", url.Values{"dish_id": {id}, "quantity": {"100"}}, 400)
	b.post("/cart/update", url.Values{"dish_id": {id}, "quantity": {"3"}}, 303)
	if !strings.Contains(b.get("/cart"), "370,35") {
		t.Fatal("incorrect cart total")
	}
	other := newBrowser(t, s)
	other.register("client2")
	other.post("/cart/update", url.Values{"dish_id": {id}, "quantity": {"9"}}, 404)
	other.post("/cart/remove", url.Values{"dish_id": {id}}, 303)
	if count(t, db, "SELECT quantity FROM cart WHERE dish_id=?", dish) != 3 {
		t.Fatal("cart isolation failed")
	}
	// Price changes are applied at checkout; snapshots remain stable afterwards.
	form.Set("id", id)
	form.Set("name", "Овсянка")
	form.Set("price", "150.25")
	a.post("/admin/dishes/save", form, 303)
	b.post("/checkout", url.Values{"comment": {"Без сахара"}}, 303)
	if count(t, db, "SELECT total FROM orders") != 45075 {
		t.Fatal("wrong checkout price")
	}
	if count(t, db, "SELECT count(*) FROM cart") != 0 {
		t.Fatal("cart not cleared")
	}
	b.post("/checkout", nil, 400)
	if !strings.Contains(b.get("/orders"), "Без сахара") {
		t.Fatal("missing order")
	}
	if strings.Contains(other.get("/orders"), "Без сахара") {
		t.Fatal("order data leak")
	}
	a.get("/admin/orders")
	a.post("/admin/orders/status", url.Values{"id": {"1"}, "status": {"completed"}}, 400)
	for _, status := range []string{"cooking", "ready", "completed"} {
		a.post("/admin/orders/status", url.Values{"id": {"1"}, "status": {status}}, 303)
	}
	a.post("/admin/orders/status", url.Values{"id": {"1"}, "status": {"new"}}, 400)
	a.post("/admin/orders/status", url.Values{"id": {"1"}, "status": {"unknown"}}, 400)
	// Add to a second cart to exercise ON DELETE CASCADE and SET NULL together.
	other.post("/cart/add", url.Values{"dish_id": {id}}, 303)
	a.post("/admin/dishes/delete", url.Values{"id": {id}}, 303)
	if count(t, db, "SELECT count(*) FROM cart") != 0 {
		t.Fatal("deleted dish retained in cart")
	}
	if count(t, db, "SELECT count(*) FROM order_items WHERE dish_id IS NULL") != 1 {
		t.Fatal("snapshot relation not cleared")
	}
	if !strings.Contains(b.get("/orders"), "Овсянка") {
		t.Fatal("historical dish lost")
	}
	a.post("/admin/categories/delete", url.Values{"id": {strconv.Itoa(cat)}}, 303)
	if count(t, db, "SELECT count(*) FROM categories WHERE id=?", cat) != 0 {
		t.Fatal("category not deleted")
	}
	a.post("/admin/dishes/delete", url.Values{"id": {"1 OR 1=1"}}, 400)
	a.post("/admin/categories/save", url.Values{"name": {strings.Repeat("Я", 81)}}, 200)
}

func TestConcurrentCheckoutAndExpiredSession(t *testing.T) {
	db, s := setup(t)
	b := newBrowser(t, s)
	b.register("parallel")
	b.post("/cart/add", url.Values{"dish_id": {"1"}}, 303)
	var wg sync.WaitGroup
	codes := make(chan int, 2)
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := b.client.PostForm(s.URL+"/checkout", url.Values{"csrf": {b.csrf}})
			if err != nil {
				codes <- 0
				return
			}
			res.Body.Close()
			codes <- res.StatusCode
		}()
	}
	wg.Wait()
	close(codes)
	seen := map[int]int{}
	for code := range codes {
		seen[code]++
	}
	if seen[303] != 1 || seen[400] != 1 || count(t, db, "SELECT count(*) FROM orders") != 1 {
		t.Fatal("duplicate checkout", seen)
	}
	if _, err := db.Exec("UPDATE sessions SET expires=0"); err != nil {
		t.Fatal(err)
	}
	b.post("/cart/add", url.Values{"dish_id": {"1"}}, 403)
	if count(t, db, "SELECT count(*) FROM cart") != 0 {
		t.Fatal("expired session accepted")
	}
}

func TestPrices(t *testing.T) {
	for input, want := range map[string]int64{"0.01": 1, "12": 1200, "12,3": 1230, "100000.00": 10000000} {
		got, err := ParsePrice(input)
		if err != nil || got != want {
			t.Errorf("%s: %d %v", input, got, err)
		}
	}
	for _, input := range []string{"-1", "0", "1.001", "1e2", "NaN", "1.2.3", "100000.01", "1.", "+1", "999999999999999999999"} {
		if _, err := ParsePrice(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
}

func TestCheckoutRollback(t *testing.T) {
	db, s := setup(t)
	b := newBrowser(t, s)
	b.register("rollback")
	b.post("/cart/add", url.Values{"dish_id": {"1"}}, 303)
	// Inject a database failure after the order insert, before the cart deletion.
	if _, err := db.Exec(`CREATE TRIGGER reject_item BEFORE INSERT ON order_items BEGIN SELECT RAISE(ABORT,'test failure'); END;`); err != nil {
		t.Fatal(err)
	}
	b.post("/checkout", nil, 500)
	if count(t, db, "SELECT count(*) FROM orders") != 0 || count(t, db, "SELECT count(*) FROM cart") != 1 {
		t.Fatal("checkout did not roll back completely")
	}
	if _, err := db.Exec("DROP TRIGGER reject_item"); err != nil {
		t.Fatal(err)
	}
	b.post("/checkout", nil, 303)
}

func TestBatchCartUpdate(t *testing.T) {
	db, s := setup(t)
	b := newBrowser(t, s)
	b.register("batch_cart")
	for _, id := range []string{"1", "2"} {
		b.post("/cart/add", url.Values{"dish_id": {id}}, 303)
	}
	body := b.get("/cart")
	if strings.Count(body, `form="cart-update" type="submit"`) != 1 {
		t.Fatal("expected a single shared cart update button")
	}
	b.post("/cart/update", url.Values{"dish_id": {"1", "2"}, "quantity": {"3", "4"}}, 303)
	if count(t, db, "SELECT quantity FROM cart WHERE dish_id=1") != 3 || count(t, db, "SELECT quantity FROM cart WHERE dish_id=2") != 4 {
		t.Fatal("batch quantities not saved")
	}
	if !strings.Contains(b.get("/cart"), "3130,00") {
		t.Fatal("batch total not recalculated")
	}
	// An invalid later row must leave all quantities unchanged.
	b.post("/cart/update", url.Values{"dish_id": {"1", "2"}, "quantity": {"8", "100"}}, 400)
	// A missing/foreign row discovered after a valid UPDATE must roll back too.
	b.post("/cart/update", url.Values{"dish_id": {"1", "3"}, "quantity": {"8", "2"}}, 404)
	b.post("/cart/update", url.Values{"dish_id": {"1", "1"}, "quantity": {"8", "2"}}, 400)
	b.post("/cart/update", url.Values{"dish_id": {"1", "2"}, "quantity": {"8"}}, 400)
	if count(t, db, "SELECT quantity FROM cart WHERE dish_id=1") != 3 || count(t, db, "SELECT quantity FROM cart WHERE dish_id=2") != 4 {
		t.Fatal("failed batch partially changed the cart")
	}
	other := newBrowser(t, s)
	other.register("other_batch")
	other.post("/cart/update", url.Values{"dish_id": {"1", "2"}, "quantity": {"1", "1"}}, 404)
	if count(t, db, "SELECT quantity FROM cart WHERE dish_id=1") != 3 {
		t.Fatal("batch update crossed user boundary")
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

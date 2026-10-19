package hostauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

const (
	// pendingLifetime is how long a Google sign-in may take.
	pendingLifetime = 10 * time.Minute
	// sessionLifetime is how long a board sign-in lasts.
	sessionLifetime = 30 * 24 * time.Hour
	// refreshLifetime is how long a connector refresh token lasts.
	refreshLifetime = 30 * 24 * time.Hour
)

// store is everything the sign-in service knows, in memory only. Client
// IDs, tokens and board cookies are sealed (see sealer), so a value from
// before a restart is taken up again when it is next used. Token-like
// values are kept under the hash of the value, so a map key never is a
// usable token. Expired entries are swept on every write.
type store struct {
	mu       sync.Mutex
	seal     *sealer
	now      func() time.Time
	pending  map[string]*pendingLogin // by key(state)
	sessions map[string]*session      // by session ID
	accounts map[string]*accountState // by Account.ID
	clients  map[string]*client       // by client ID
	codes    map[string]*authCode     // by key(code)
	refresh  map[string]*refreshToken // by refresh token ID
	// ended and used hold, until they expire, the board sessions signed
	// out and the refresh tokens already exchanged, which must not be
	// taken up again.
	ended   map[string]time.Time
	used    map[string]time.Time
	buckets map[string]*bucket // by Account.ID
	// onRelease, if set, is told when an account's last live sign-in
	// ends. It is called with mu held, so it must not call back into the
	// store.
	onRelease func(id string)
}

func newStore(seal *sealer, now func() time.Time) *store {
	return &store{
		seal:     seal,
		now:      now,
		pending:  map[string]*pendingLogin{},
		sessions: map[string]*session{},
		accounts: map[string]*accountState{},
		clients:  map[string]*client{},
		codes:    map[string]*authCode{},
		refresh:  map[string]*refreshToken{},
		ended:    map[string]time.Time{},
		used:     map[string]time.Time{},
		buckets:  map[string]*bucket{},
	}
}

// loginKind says how a Google sign-in continues once Google answers.
type loginKind int

const (
	// loginBoard returns the browser to the board page it asked for.
	loginBoard loginKind = iota
	// loginConnector returns an authorization code to an MCP client.
	loginConnector
)

// pendingLogin is a Google sign-in in progress, tied to one browser by the
// login cookie.
type pendingLogin struct {
	kind     loginKind
	next     string // loginBoard: the local path to return to
	nonce    string // the ID token must carry it
	verifier string // PKCE verifier for the Google code exchange
	expires  time.Time

	// loginConnector: the authorization request being answered.
	clientID, redirect, clientState, challenge string
}

// session is a browser's board sign-in.
type session struct {
	account Account
	expires time.Time
	// done is closed when the session ends, so live connections opened
	// under it can stop.
	done chan struct{}
}

// accountState is a signed-in account and how many live sign-ins (board
// sessions and connector refresh tokens) it holds.
type accountState struct {
	email string
	live  int
}

// newToken returns a fresh random value for a token, code or identifier.
func newToken() string { return rand.Text() }

// key returns the map key for a token-like value.
func key(v string) string {
	h := sha256.Sum256([]byte(v))
	return hex.EncodeToString(h[:])
}

// sweep deletes expired entries. The caller holds mu.
func (s *store) sweep(now time.Time) {
	for k, p := range s.pending {
		if now.After(p.expires) {
			delete(s.pending, k)
		}
	}
	for k, c := range s.codes {
		if now.After(c.expires) {
			delete(s.codes, k)
			s.releaseLocked(c.account.ID)
		}
	}
	for id, ss := range s.sessions {
		if now.After(ss.expires) {
			delete(s.sessions, id)
			close(ss.done)
			s.releaseLocked(ss.account.ID)
		}
	}
	for id, t := range s.refresh {
		if now.After(t.expires) {
			delete(s.refresh, id)
			s.releaseLocked(t.account.ID)
		}
	}
	for _, m := range []map[string]time.Time{s.ended, s.used} {
		for id, exp := range m {
			if now.After(exp) {
				delete(m, id)
			}
		}
	}
	// A bucket untouched for a minute is full again: forget it.
	for id, b := range s.buckets {
		if now.Sub(b.last) >= time.Minute {
			delete(s.buckets, id)
		}
	}
}

// addPending records a sign-in in progress and returns its state value.
func (s *store) addPending(now time.Time, p *pendingLogin) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	state := newToken()
	p.expires = now.Add(pendingLifetime)
	s.pending[key(state)] = p
	return state
}

// takePending removes and returns the live sign-in in progress for state.
func (s *store) takePending(now time.Time, state string) *pendingLogin {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(state)
	p := s.pending[k]
	delete(s.pending, k)
	if p == nil || now.After(p.expires) {
		return nil
	}
	return p
}

// addSession starts a board session for acct and returns its cookie value.
func (s *store) addSession(now time.Time, acct Account) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	id := newToken()
	exp := now.Add(sessionLifetime)
	s.sessions[id] = &session{account: acct, expires: exp, done: make(chan struct{})}
	s.holdLocked(acct)
	return s.seal.seal(sealSession, grantClaims{ID: id, Sub: acct.ID, Email: acct.Email, Exp: exp.Unix()})
}

// session returns the board session for a cookie value, or nil. A cookie
// from before a restart starts its session again.
func (s *store) session(cookie string) *session {
	var g grantClaims
	if !s.seal.open(sealSession, cookie, &g) || g.ID == "" {
		return nil
	}
	exp := time.Unix(g.Exp, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now().After(exp) {
		return nil
	}
	if ss := s.sessions[g.ID]; ss != nil {
		return ss
	}
	if _, out := s.ended[g.ID]; out {
		return nil
	}
	ss := &session{account: g.account(), expires: exp, done: make(chan struct{})}
	s.sessions[g.ID] = ss
	s.holdLocked(ss.account)
	return ss
}

// endSession ends the board session of a cookie value: its live
// connections stop, its account holds one fewer sign-in, and the cookie is
// not taken up again while this instance runs.
func (s *store) endSession(cookie string) {
	var g grantClaims
	if !s.seal.open(sealSession, cookie, &g) || g.ID == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ended[g.ID] = time.Unix(g.Exp, 0)
	ss := s.sessions[g.ID]
	if ss == nil {
		return
	}
	delete(s.sessions, g.ID)
	close(ss.done)
	s.releaseLocked(ss.account.ID)
}

// holdLocked counts one more live sign-in for acct. The caller holds mu.
func (s *store) holdLocked(acct Account) {
	st := s.accounts[acct.ID]
	if st == nil {
		st = &accountState{}
		s.accounts[acct.ID] = st
	}
	st.email = acct.Email
	st.live++
}

// releaseLocked counts one fewer live sign-in for the account, forgetting
// it at zero and telling onRelease. The caller holds mu.
func (s *store) releaseLocked(id string) {
	st := s.accounts[id]
	if st == nil {
		return
	}
	st.live--
	if st.live <= 0 {
		delete(s.accounts, id)
		if s.onRelease != nil {
			s.onRelease(id)
		}
	}
}

// client is a registered MCP client.
type client struct {
	id         string
	secretKey  string // key(secret), or "" for a public client
	redirects  []string
	authMethod string
	created    time.Time
}

// authCode is a one-time authorization code.
type authCode struct {
	clientID, redirect, challenge string
	account                       Account
	expires                       time.Time
}

// accessToken is a connector access token.
type accessToken struct {
	account Account
	expires time.Time
}

// refreshToken is a connector refresh token; it lasts until it is used or
// expires, and holds a live sign-in for its account.
type refreshToken struct {
	account Account
	expires time.Time
}

// newClient returns a client with a sealed ID, so it is still known after
// a restart.
func (s *store) newClient(now time.Time, redirects []string, method, secretKey string) *client {
	c := &client{secretKey: secretKey, redirects: redirects, authMethod: method, created: now}
	c.id = s.seal.seal(sealClient, clientClaims{Redirects: redirects, Method: method, SecretKey: secretKey, Created: now.Unix()})
	return c
}

// addClient registers c, forgetting the oldest client when full.
func (s *store) addClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addClientLocked(c)
}

func (s *store) addClientLocked(c *client) {
	if len(s.clients) >= maxClients {
		var oldest *client
		for _, o := range s.clients {
			if oldest == nil || o.created.Before(oldest.created) {
				oldest = o
			}
		}
		delete(s.clients, oldest.id)
	}
	s.clients[c.id] = c
}

// client returns the registered client with id, or nil. A client
// registered before a restart is known again from its ID.
func (s *store) client(id string) *client {
	s.mu.Lock()
	defer s.mu.Unlock()
	if c := s.clients[id]; c != nil {
		return c
	}
	var cc clientClaims
	if !s.seal.open(sealClient, id, &cc) {
		return nil
	}
	c := &client{id: id, secretKey: cc.SecretKey, redirects: cc.Redirects, authMethod: cc.Method, created: time.Unix(cc.Created, 0)}
	s.addClientLocked(c)
	return c
}

// addCode records a one-time code and returns it. Until it is exchanged
// or expires, the code holds a live sign-in for its account, so the
// account keeps its place under the user cap even if its board session
// ends first.
func (s *store) addCode(now time.Time, c *authCode) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	code := newToken()
	c.expires = now.Add(codeLifetime)
	s.codes[key(code)] = c
	s.holdLocked(c.account)
	return code
}

// takeCode removes and returns the code, or nil. A code is gone after any
// attempt to use it. The caller passes the code's live sign-in on to the
// tokens it issues (issueTokens), or gives it back with release.
func (s *store) takeCode(code string) *authCode {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(code)
	c := s.codes[k]
	delete(s.codes, k)
	return c
}

// takeRefresh uses up the refresh token if it was issued to clientID and
// has not expired, and returns its account. Its live sign-in passes,
// without a gap, to the tokens issued next (issueTokens); a token from
// before a restart takes up a live sign-in for them.
func (s *store) takeRefresh(clientID, token string) (Account, bool) {
	var g grantClaims
	if !s.seal.open(sealRefresh, token, &g) || g.ID == "" || g.Client != clientKey(clientID) {
		return Account{}, false
	}
	exp := time.Unix(g.Exp, 0)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.now().After(exp) {
		return Account{}, false
	}
	if _, done := s.used[g.ID]; done {
		return Account{}, false
	}
	s.used[g.ID] = exp
	if t := s.refresh[g.ID]; t != nil {
		delete(s.refresh, g.ID)
		return t.account, true
	}
	s.holdLocked(g.account())
	return g.account(), true
}

// release gives back a live sign-in taken from a code that was not
// exchanged.
func (s *store) release(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.releaseLocked(id)
}

// issueTokens issues an access and a refresh token to clientID for acct.
// The refresh token takes over the live sign-in of the code or refresh
// token just exchanged, so the account never drops out of the count.
func (s *store) issueTokens(now time.Time, clientID string, acct Account) (access, refresh string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sweep(now)
	ck := clientKey(clientID)
	access = s.seal.seal(sealAccess, grantClaims{Client: ck, Sub: acct.ID, Email: acct.Email, Exp: now.Add(accessLifetime).Unix()})
	id, exp := newToken(), now.Add(refreshLifetime)
	s.refresh[id] = &refreshToken{account: acct, expires: exp}
	refresh = s.seal.seal(sealRefresh, grantClaims{ID: id, Client: ck, Sub: acct.ID, Email: acct.Email, Exp: exp.Unix()})
	return access, refresh
}

// accessToken returns the access token's record, or nil. Access tokens
// are sealed and never stored, so one from before a restart still works.
func (s *store) accessToken(token string) *accessToken {
	var g grantClaims
	if !s.seal.open(sealAccess, token, &g) {
		return nil
	}
	return &accessToken{account: g.account(), expires: time.Unix(g.Exp, 0)}
}

package hostauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"
)

// pendingLifetime is how long a Google sign-in may take. Board sessions
// and refresh tokens last until the process ends.
const pendingLifetime = 10 * time.Minute

// store is everything the sign-in service knows, in memory only. Token-like
// values are kept under the hash of the value, so a map key never is a
// usable token. Expired entries are swept on every write.
type store struct {
	mu       sync.Mutex
	pending  map[string]*pendingLogin // by key(state)
	sessions map[string]*session      // by key(cookie value)
	accounts map[string]*accountState // by Account.ID
	clients  map[string]*client       // by client ID
	codes    map[string]*authCode     // by key(code)
	access   map[string]*accessToken  // by key(token)
	refresh  map[string]*refreshToken // by key(token)
	buckets  map[string]*bucket       // by Account.ID
	// onRelease, if set, is told when an account's last live sign-in
	// ends. It is called with mu held, so it must not call back into the
	// store.
	onRelease func(id string)
}

func newStore() *store {
	return &store{
		pending:  map[string]*pendingLogin{},
		sessions: map[string]*session{},
		accounts: map[string]*accountState{},
		clients:  map[string]*client{},
		codes:    map[string]*authCode{},
		access:   map[string]*accessToken{},
		refresh:  map[string]*refreshToken{},
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
	for k, t := range s.access {
		if now.After(t.expires) {
			delete(s.access, k)
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
	s.sessions[key(id)] = &session{account: acct, done: make(chan struct{})}
	s.holdLocked(acct)
	return id
}

// session returns the board session for a cookie value, or nil.
func (s *store) session(id string) *session {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sessions[key(id)]
}

// endSession ends a board session: its live connections stop and its
// account holds one fewer sign-in.
func (s *store) endSession(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(id)
	ss := s.sessions[k]
	if ss == nil {
		return
	}
	delete(s.sessions, k)
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
	clientID string
	account  Account
	expires  time.Time
}

// refreshToken is a connector refresh token; it lasts until it is used or
// the process ends, and holds a live sign-in for its account.
type refreshToken struct {
	clientID string
	account  Account
}

// addClient registers c, forgetting the oldest client when full.
func (s *store) addClient(c *client) {
	s.mu.Lock()
	defer s.mu.Unlock()
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

// client returns the registered client with id, or nil.
func (s *store) client(id string) *client {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.clients[id]
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

// takeRefresh removes the refresh token if it was issued to clientID and
// returns its account. Its live sign-in passes, without a gap, to the
// tokens issued next (issueTokens).
func (s *store) takeRefresh(clientID, token string) (Account, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := key(token)
	t := s.refresh[k]
	if t == nil || t.clientID != clientID {
		return Account{}, false
	}
	delete(s.refresh, k)
	return t.account, true
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
	access, refresh = newToken(), newToken()
	s.access[key(access)] = &accessToken{clientID: clientID, account: acct, expires: now.Add(accessLifetime)}
	s.refresh[key(refresh)] = &refreshToken{clientID: clientID, account: acct}
	return access, refresh
}

// accessToken returns the access token's record, or nil.
func (s *store) accessToken(token string) *accessToken {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.access[key(token)]
}

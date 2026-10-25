package tcp

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/eqoa/server/internal/auth"
	"github.com/eqoa/server/internal/database"
)

// AccountServer provides HTTP endpoints for account registration and character management.
type AccountServer struct {
	db           *database.SQLiteDB
	accountStore *database.SQLiteAccountStore
	charStore    *database.SQLiteCharacterStore
	defaults     *database.DefaultCharacterStore
	port         int
	bcryptCost   int
	server       *http.Server

	// Track IPs that have created accounts
	createdIPs   map[string]bool
	createdIPsMu sync.RWMutex

	// Session tokens for logged-in users
	sessions   map[string]*webSession
	sessionsMu sync.RWMutex

	// TELEPORT, wired by main.go. The account pages can move a character, which
	// is the game's business rather than the web server's, so the two things it
	// cannot work out for itself are handed in.
	//
	// Destinations returns the same named shortcuts #tele offers, and Resolve
	// answers where one actually IS — which depends on where the defining NPC
	// spawned, so it cannot be a table over here.
	//
	// TeleportLive moves a character who is logged in and reports whether it
	// found one; false means "not online", and the caller writes the database
	// row instead. Both nil on a server with no game world attached, which
	// disables the route rather than half-performing it.
	//
	// SendOnly names the worlds that may be SENT to and never SAVED. The HDD
	// setup screen is one: it replaces the game rather than loading a world, so
	// a character whose stored world is that index launches it again on every
	// login, forever, with no error at either end. TeleportLive can refuse it
	// for an offline character by returning false — but false means "write the
	// row", which is the very thing that must not happen, so the question has to
	// be asked separately. Nil means every world is savable.
	//
	// CanLoad refuses a world the character's game cannot load: its session's
	// table while it is logged in, else the disc's six that a login starts in
	// (handlers.LoadableWorlds). Asked before either move, because a client
	// told to load a world its table lacks stays where it was while the
	// server files the character under that world. Nil allows every world.
	Destinations  func() []TeleportDest
	WorldNames    func() []string
	ResolveDest   func(name string) (x, y, z float32, world byte, via string, ok bool)
	ValidatePlace func(x, y, z float32, world byte) (byte, error)
	TeleportLive  func(serverID uint32, x, y, z float32, world byte) bool
	SendOnly      func(world byte) bool
	CanLoad       func(serverID uint32, world byte) error

	// EndSessions ends every session an account has in the game (each is
	// told 0x07A4, "You have logged in from another location.") and returns
	// how many: /api/logout, the browser's way past "You are already logged
	// in." (session.Manager.EndAccount).
	EndSessions func(accountID int) int

	// Limiter counts the password checks of /api/login and /api/logout: the
	// server's one limiter, shared with the game's login and the GM panel, so
	// guesses spread over them add up. Nil: not counted.
	Limiter *auth.Limiter
}

// checkPassword is a page's password check under the limiter: counted by the
// client's address and the name before it is made, the attempt given back
// when it is right. refusal is what to tell the client when it is refused
// before any check; "" otherwise.
func (s *AccountServer) checkPassword(r *http.Request, account *database.AccountRecord, username, password string) (ok bool, refusal string) {
	ip := s.getClientIP(r)
	if s.Limiter != nil {
		if wait := s.Limiter.Begin(ip, username); wait > 0 {
			log.Printf("AccountServer: refused '%s' from %s: too many failed logins", username, ip)
			return false, fmt.Sprintf("Too many failed logins; try again in %d minutes", auth.RetryMinutes(wait))
		}
		release, free := s.Limiter.Slot(10 * time.Second)
		if !free {
			return false, "The server is busy; try again in a moment"
		}
		defer release()
	}
	if account == nil {
		return false, ""
	}
	switch auth.VerifyPassword(account.PasswordHash, password) {
	case auth.Accepted:
	case auth.AcceptedNeedsRehash:
		// Stored plaintext, from before this page hashed anything. It matched,
		// so replace it now -- this is the only point the plaintext is known.
		if hash, herr := auth.HashPassword(password, s.bcryptCost); herr == nil {
			if uerr := s.accountStore.UpdatePasswordHash(account.ID, hash); uerr != nil {
				log.Printf("AccountServer: could not upgrade stored password for '%s': %v", account.Username, uerr)
			} else {
				log.Printf("AccountServer: upgraded '%s' from a plaintext password to bcrypt", account.Username)
			}
		}
	default:
		// Covers Rejected and NoCredential. An account with no password set
		// must not be loggable into with an empty one.
		return false, ""
	}
	if s.Limiter != nil {
		s.Limiter.Succeed(ip, username)
	}
	return true, ""
}

// TeleportDest is one named shortcut as the web pages present it.
type TeleportDest struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	World byte   `json:"world"`
}

type webSession struct {
	AccountID  int
	Username   string
	CreatedAt  time.Time
	LastAccess time.Time
}

// NewAccountServer creates a new account registration server.
func NewAccountServer(db *database.SQLiteDB, port int, bcryptCost int) *AccountServer {
	s := &AccountServer{
		db:           db,
		accountStore: database.NewSQLiteAccountStore(db),
		charStore:    database.NewSQLiteCharacterStore(db),
		defaults:     database.NewDefaultCharacterStore(db),
		port:         port,
		bcryptCost:   bcryptCost,
		createdIPs:   make(map[string]bool),
		sessions:     make(map[string]*webSession),
	}

	// Start session cleanup goroutine
	go s.cleanupSessions()

	return s
}

// cleanupSessions removes expired sessions periodically.
func (s *AccountServer) cleanupSessions() {
	ticker := time.NewTicker(5 * time.Minute)
	for range ticker.C {
		s.sessionsMu.Lock()
		now := time.Now()
		for token, sess := range s.sessions {
			if now.Sub(sess.LastAccess) > 30*time.Minute {
				delete(s.sessions, token)
			}
		}
		s.sessionsMu.Unlock()
	}
}

// Start starts the account registration server.
func (s *AccountServer) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/register", s.handleRegisterPage)
	mux.HandleFunc("/login", s.handleLoginPage)
	mux.HandleFunc("/characters", s.handleCharactersPage)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/api/register", s.handleAPIRegister)
	mux.HandleFunc("/api/login", s.handleAPILogin)
	mux.HandleFunc("/api/characters", s.handleAPICharacters)
	mux.HandleFunc("/api/characters/create", s.handleAPICreateCharacter)
	mux.HandleFunc("/api/characters/delete", s.handleAPIDeleteCharacter)
	mux.HandleFunc("/api/characters/teleport", s.handleAPITeleport)
	mux.HandleFunc("/api/teleport/destinations", s.handleAPITeleportDests)
	mux.HandleFunc("/api/creation", s.handleAPICreation)
	mux.HandleFunc("/api/logout", s.handleAPILogoutSessions)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	go func() {
		log.Printf("Account registration server starting on port %d", s.port)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("Account server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the account server.
func (s *AccountServer) Stop() {
	if s.server != nil {
		s.server.Close()
	}
}

// getClientIP extracts the client IP from the request. Behind a proxy on
// loopback (nginx), the client is the address the proxy names: X-Real-IP,
// else the LAST X-Forwarded-For entry (the proxy appends to whatever the
// client sent, so the first is the client's to choose). From any other peer
// the headers are ignored.
func (s *AccountServer) getClientIP(r *http.Request) string {
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	if p := net.ParseIP(ip); p != nil && p.IsLoopback() {
		if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
			return real
		}
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			return strings.TrimSpace(parts[len(parts)-1])
		}
	}
	return ip
}

// handleAPICreation lists what character creation offers: one entry per
// (race, city, class) in the defaultcharacter table, which holds one row per
// class a race's classMask allows (claude-notes/new-race-checklist.md), with
// the starting stats (60 plus the race's and the class's bonuses). A new
// character spends `points` over them.
func (s *AccountServer) handleAPICreation(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	type combo struct {
		Race  int    `json:"race"`
		City  int    `json:"city"`
		Class int    `json:"class"`
		Stats [7]int `json:"stats"`
	}
	out := []combo{}
	for _, d := range s.defaults.All() {
		out = append(out, combo{d.Race, d.HumType, d.TClass, [7]int{d.DefaultStrength, d.DefaultStamina,
			d.DefaultAgility, d.DefaultDexterity, d.DefaultWisdom, d.DefaultIntelligence, d.DefaultCharisma}})
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"combos": out, "points": 20})
}

// generateToken creates a random session token.
func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// getSession retrieves a session from cookie.
func (s *AccountServer) getSession(r *http.Request) *webSession {
	cookie, err := r.Cookie("session")
	if err != nil {
		return nil
	}
	s.sessionsMu.RLock()
	sess := s.sessions[cookie.Value]
	s.sessionsMu.RUnlock()
	if sess != nil {
		sess.LastAccess = time.Now()
	}
	return sess
}

// createSession creates a new session for an account.
func (s *AccountServer) createSession(w http.ResponseWriter, accountID int, username string) string {
	token := generateToken()
	s.sessionsMu.Lock()
	s.sessions[token] = &webSession{
		AccountID:  accountID,
		Username:   username,
		CreatedAt:  time.Now(),
		LastAccess: time.Now(),
	}
	s.sessionsMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		MaxAge:   1800, // 30 minutes
		HttpOnly: true,
	})
	return token
}

// CSS styles shared across pages
const cssStyles = `
<style>
    * { box-sizing: border-box; }
    body {
        font-family: 'Segoe UI', Arial, sans-serif;
        max-width: 800px;
        margin: 0 auto;
        padding: 20px;
        background: #1a1a2e;
        color: #eee;
        min-height: 100vh;
    }
    h1, h2 {
        color: #e94560;
        text-align: center;
    }
    .nav {
        display: flex;
        justify-content: center;
        gap: 20px;
        margin-bottom: 30px;
        padding: 10px;
        background: #16213e;
        border-radius: 8px;
    }
    .nav a {
        color: #e94560;
        text-decoration: none;
        padding: 8px 16px;
        border-radius: 4px;
        transition: background 0.2s;
    }
    .nav a:hover {
        background: #0f3460;
    }
    .nav .user {
        color: #aaa;
        margin-left: auto;
    }
    .form-group {
        margin-bottom: 15px;
    }
    label {
        display: block;
        margin-bottom: 5px;
        color: #aaa;
    }
    input[type="text"], input[type="password"], input[type="number"], select {
        width: 100%;
        padding: 10px;
        border: 1px solid #0f3460;
        border-radius: 4px;
        background: #16213e;
        color: #eee;
    }
    button, .btn {
        padding: 10px 20px;
        background: #e94560;
        color: white;
        border: none;
        border-radius: 4px;
        cursor: pointer;
        font-size: 14px;
        text-decoration: none;
        display: inline-block;
    }
    button:hover, .btn:hover {
        background: #ff6b6b;
    }
    .btn-danger {
        background: #c0392b;
    }
    .btn-danger:hover {
        background: #e74c3c;
    }
    .btn-small {
        padding: 5px 10px;
        font-size: 12px;
    }
    .message {
        padding: 10px;
        border-radius: 4px;
        margin-bottom: 15px;
        text-align: center;
    }
    .error { background: #c0392b; }
    .success { background: #27ae60; }
    .card {
        background: #16213e;
        border-radius: 8px;
        padding: 20px;
        margin-bottom: 20px;
    }
    .char-list {
        display: grid;
        gap: 15px;
    }
    .loc { color: #8a8aa0; font-size: 12px; margin: 4px 0; }
    .tele-row { display: flex; gap: 8px; align-items: center; margin-top: 8px; }
    .tele-row select { flex: 1; padding: 6px; }
    .tele-custom { display: flex; gap: 6px; margin-top: 6px; }
    .tele-custom input { width: 25%; padding: 6px; }
    .char-card {
        background: #0f3460;
        border-radius: 8px;
        padding: 15px;
        display: flex;
        justify-content: space-between;
        align-items: center;
    }
    .char-info h3 {
        margin: 0 0 5px 0;
        color: #e94560;
    }
    .char-info p {
        margin: 0;
        color: #aaa;
        font-size: 14px;
    }
    .char-actions {
        display: flex;
        gap: 10px;
    }
    table {
        width: 100%;
        border-collapse: collapse;
    }
    th, td {
        padding: 8px;
        text-align: left;
        border-bottom: 1px solid #0f3460;
    }
    th { color: #e94560; }
    .info {
        background: #16213e;
        color: #aaa;
        font-size: 12px;
        text-align: center;
        padding: 10px;
        border-radius: 4px;
        margin-top: 20px;
    }
    .form-row {
        display: grid;
        grid-template-columns: 1fr 1fr;
        gap: 15px;
    }
    @media (max-width: 600px) {
        .form-row { grid-template-columns: 1fr; }
        .char-card { flex-direction: column; gap: 10px; }
    }
</style>`

// handleIndex serves the main page.
func (s *AccountServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	sess := s.getSession(r)
	if sess != nil {
		http.Redirect(w, r, "/characters", http.StatusSeeOther)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>EQOA Server</title>
    ` + cssStyles + `
</head>
<body>
    <h1>EQOA Private Server</h1>
    <div class="nav">
        <a href="/login">Login</a>
        <a href="/register">Register</a>
    </div>
    <div class="card">
        <h2>Welcome</h2>
        <p style="text-align: center; color: #aaa;">
            Register a new account or login to manage your characters.
        </p>
    </div>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleRegisterPage serves the registration form.
func (s *AccountServer) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html>
<head>
    <title>Register - EQOA</title>
    ` + cssStyles + `
</head>
<body>
    <h1>Create Account</h1>
    <div class="nav">
        <a href="/">Home</a>
        <a href="/login">Login</a>
        <a href="/register">Register</a>
    </div>
    <div class="card">
        <div id="message"></div>
        <form id="registerForm">
            <div class="form-group">
                <label for="username">Username (3-16 characters, letters/numbers only)</label>
                <input type="text" id="username" name="username" required minlength="3" maxlength="16" pattern="[a-zA-Z0-9_]+">
            </div>
            <div class="form-group">
                <label for="password">Password (4+ characters)</label>
                <input type="password" id="password" name="password" required minlength="4">
            </div>
            <div class="form-group">
                <label for="confirm">Confirm Password</label>
                <input type="password" id="confirm" name="confirm" required>
            </div>
            <button type="submit" style="width:100%">Create Account</button>
        </form>
    </div>
    <div class="info">One account per IP address.</div>
    <script>
        document.getElementById('registerForm').addEventListener('submit', async function(e) {
            e.preventDefault();
            const msg = document.getElementById('message');
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            const confirm = document.getElementById('confirm').value;
            if (password !== confirm) {
                msg.className = 'message error';
                msg.textContent = 'Passwords do not match';
                return;
            }
            try {
                const response = await fetch('/api/register', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({username, password})
                });
                const data = await response.json();
                if (data.success) {
                    msg.className = 'message success';
                    msg.textContent = 'Account created! Redirecting to login...';
                    setTimeout(() => window.location.href = '/login', 1500);
                } else {
                    msg.className = 'message error';
                    msg.textContent = data.error || 'Registration failed';
                }
            } catch (err) {
                msg.className = 'message error';
                msg.textContent = 'Connection error';
            }
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleLoginPage serves the login form.
func (s *AccountServer) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r)
	if sess != nil {
		http.Redirect(w, r, "/characters", http.StatusSeeOther)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Login - EQOA</title>
    ` + cssStyles + `
</head>
<body>
    <h1>Login</h1>
    <div class="nav">
        <a href="/">Home</a>
        <a href="/login">Login</a>
        <a href="/register">Register</a>
    </div>
    <div class="card">
        <div id="message"></div>
        <form id="loginForm">
            <div class="form-group">
                <label for="username">Username</label>
                <input type="text" id="username" name="username" required>
            </div>
            <div class="form-group">
                <label for="password">Password</label>
                <input type="password" id="password" name="password" required>
            </div>
            <button type="submit" style="width:100%">Login</button>
        </form>
    </div>
    <script>
        document.getElementById('loginForm').addEventListener('submit', async function(e) {
            e.preventDefault();
            const msg = document.getElementById('message');
            const username = document.getElementById('username').value;
            const password = document.getElementById('password').value;
            try {
                const response = await fetch('/api/login', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({username, password})
                });
                const data = await response.json();
                if (data.success) {
                    window.location.href = '/characters';
                } else {
                    msg.className = 'message error';
                    msg.textContent = data.error || 'Login failed';
                }
            } catch (err) {
                msg.className = 'message error';
                msg.textContent = 'Connection error';
            }
        });
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleCharactersPage serves the character management page.
func (s *AccountServer) handleCharactersPage(w http.ResponseWriter, r *http.Request) {
	sess := s.getSession(r)
	if sess == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	html := `<!DOCTYPE html>
<html>
<head>
    <title>Characters - EQOA</title>
    ` + cssStyles + `
</head>
<body>
    <h1>My Characters</h1>
    <div class="nav">
        <a href="/characters">Characters</a>
        <a href="/logout">Logout</a>
        <span class="user">Logged in as: ` + sess.Username + `</span>
    </div>
    <div id="message"></div>
    <div class="card">
        <h2>Your Characters</h2>
        <div id="charList" class="char-list">Loading...</div>
    </div>
    <div class="card">
        <h2>Create New Character</h2>
        <form id="createForm">
            <div class="form-row">
                <div class="form-group">
                    <label for="name">Name (3-15 letters)</label>
                    <input type="text" id="name" name="name" required minlength="3" maxlength="15" pattern="[a-zA-Z]+">
                </div>
                <div class="form-group">
                    <label for="race">Race</label>
                    <select id="race" name="race" required>
                        <option value="0">Human</option>
                        <option value="1">Elf</option>
                        <option value="2">Dark Elf</option>
                        <option value="3">Gnome</option>
                        <option value="4">Halfling</option>
                        <option value="5">Dwarf</option>
                        <option value="6">Troll</option>
                        <option value="7">Ogre</option>
                        <option value="8">Barbarian</option>
                        <option value="9">Erudite</option>
                    </select>
                </div>
            </div>
            <div class="form-row">
                <div class="form-group">
                    <label for="class">Class</label>
                    <select id="class" name="class" required>
                        <option value="0">Warrior</option>
                        <option value="1">Ranger</option>
                        <option value="2">Paladin</option>
                        <option value="3">Shadow Knight</option>
                        <option value="4">Monk</option>
                        <option value="5">Bard</option>
                        <option value="6">Rogue</option>
                        <option value="7">Alchemist</option>
                        <option value="8">Druid</option>
                        <option value="9">Shaman</option>
                        <option value="10">Cleric</option>
                        <option value="11">Magician</option>
                        <option value="12">Enchanter</option>
                        <option value="13">Wizard</option>
                        <option value="14">Necromancer</option>
                    </select>
                </div>
                <div class="form-group">
                    <label for="gender">Gender</label>
                    <select id="gender" name="gender" required>
                        <option value="0">Male</option>
                        <option value="1">Female</option>
                    </select>
                </div>
            </div>
            <button type="submit">Create Character</button>
        </form>
    </div>
    <script>
        const races = ['Human', 'Elf', 'Dark Elf', 'Gnome', 'Halfling', 'Dwarf', 'Troll', 'Ogre', 'Barbarian', 'Erudite'];
        const classes = ['Warrior', 'Ranger', 'Paladin', 'Shadow Knight', 'Monk', 'Bard', 'Rogue', 'Alchemist', 'Druid', 'Shaman', 'Cleric', 'Magician', 'Enchanter', 'Wizard', 'Necromancer'];

        async function loadCharacters() {
            try {
                const response = await fetch('/api/characters');
                const data = await response.json();
                const list = document.getElementById('charList');
                if (!data.characters || data.characters.length === 0) {
                    list.innerHTML = '<p style="color:#aaa;text-align:center">No characters yet. Create one below!</p>';
                    return;
                }
                list.innerHTML = data.characters.map(c => ` + "`" + `
                    <div class="char-card">
                        <div class="char-info">
                            <h3>${c.name}</h3>
                            <p>Level ${c.level} ${races[c.race] || 'Unknown'} ${classes[c.class] || 'Unknown'}</p>
                            <p class="loc">Last saved: ${worldName(c.world)} (${Math.round(c.x)}, ${Math.round(c.y)}, ${Math.round(c.z)})</p>
                            <div class="tele-row">
                                <select id="dest-${c.id}" onchange="toggleCustom(${c.id})">
                                    ${destOptions()}
                                    <option value="">Custom coordinates...</option>
                                </select>
                                <button class="btn btn-small" onclick="teleportChar(${c.id}, '${c.name}')">Teleport</button>
                            </div>
                            <div class="tele-custom" id="custom-${c.id}" style="display:none">
                                <input type="number" id="w-${c.id}" placeholder="world" value="${c.world}">
                                <input type="number" id="x-${c.id}" placeholder="x" value="${Math.round(c.x)}">
                                <input type="number" id="y-${c.id}" placeholder="y" value="${Math.round(c.y)}">
                                <input type="number" id="z-${c.id}" placeholder="z" value="${Math.round(c.z)}">
                            </div>
                        </div>
                        <div class="char-actions">
                            <button class="btn btn-danger btn-small" onclick="deleteChar(${c.id}, '${c.name}')">Delete</button>
                        </div>
                    </div>
                ` + "`" + `).join('');
            } catch (err) {
                document.getElementById('charList').innerHTML = '<p style="color:#e94560">Failed to load characters</p>';
            }
        }

        // Destinations come from the server so the page cannot offer a place the
        // game does not have. Loaded once, before the first character list.
        let destinations = [];
        let worldNames = [];

        function destOptions() {
            return destinations.map(d =>
                '<option value="' + d.name + '">' + d.label + '</option>').join('');
        }

        function worldName(w) {
            return worldNames[w] ? worldNames[w] + ' (' + w + ')' : 'world ' + w;
        }

        function toggleCustom(id) {
            const custom = document.getElementById('dest-' + id).value === '';
            document.getElementById('custom-' + id).style.display = custom ? 'flex' : 'none';
        }

        async function loadDests() {
            try {
                const r = await fetch('/api/teleport/destinations');
                const d = await r.json();
                destinations = d.destinations || [];
                worldNames = d.worlds || [];
            } catch (err) {
                destinations = [];
            }
        }

        async function teleportChar(id, name) {
            const msg = document.getElementById('message');
            const dest = document.getElementById('dest-' + id).value;
            const body = {serverid: id, dest};
            if (!dest) {
                body.world = parseInt(document.getElementById('w-' + id).value, 10) || 0;
                body.x = parseFloat(document.getElementById('x-' + id).value) || 0;
                body.y = parseFloat(document.getElementById('y-' + id).value) || 0;
                body.z = parseFloat(document.getElementById('z-' + id).value) || 0;
            }
            try {
                const r = await fetch('/api/characters/teleport', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(body)
                });
                const data = await r.json();
                if (data.success) {
                    msg.className = 'message success';
                    // Say WHICH of the two happened. "Moved" means something
                    // different for a character standing in the world than for a
                    // row that takes effect at next login, and a player who is
                    // not told will go looking for the wrong one.
                    msg.textContent = name + ' moved to ' + data.via + ' (' +
                        Math.round(data.x) + ', ' + Math.round(data.y) + ', ' +
                        Math.round(data.z) + ') world ' + data.world +
                        (data.online ? ' - in game now' : ' - takes effect at next login');
                    loadDests().then(loadCharacters);
                } else {
                    msg.className = 'message error';
                    msg.textContent = data.error || 'Teleport failed';
                }
            } catch (err) {
                msg.className = 'message error';
                msg.textContent = 'Connection error';
            }
            setTimeout(() => msg.className = '', 5000);
        }

        async function deleteChar(id, name) {
            if (!confirm('Are you sure you want to delete ' + name + '? This cannot be undone!')) return;
            const msg = document.getElementById('message');
            try {
                const response = await fetch('/api/characters/delete', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify({id})
                });
                const data = await response.json();
                if (data.success) {
                    msg.className = 'message success';
                    msg.textContent = 'Character deleted';
                    loadCharacters();
                } else {
                    msg.className = 'message error';
                    msg.textContent = data.error || 'Delete failed';
                }
            } catch (err) {
                msg.className = 'message error';
                msg.textContent = 'Connection error';
            }
            setTimeout(() => msg.className = '', 3000);
        }

        document.getElementById('createForm').addEventListener('submit', async function(e) {
            e.preventDefault();
            const msg = document.getElementById('message');
            const formData = {
                name: document.getElementById('name').value,
                race: parseInt(document.getElementById('race').value),
                class: parseInt(document.getElementById('class').value),
                gender: parseInt(document.getElementById('gender').value)
            };
            try {
                const response = await fetch('/api/characters/create', {
                    method: 'POST',
                    headers: {'Content-Type': 'application/json'},
                    body: JSON.stringify(formData)
                });
                const data = await response.json();
                if (data.success) {
                    msg.className = 'message success';
                    msg.textContent = 'Character created!';
                    document.getElementById('createForm').reset();
                    loadCharacters();
                } else {
                    msg.className = 'message error';
                    msg.textContent = data.error || 'Creation failed';
                }
            } catch (err) {
                msg.className = 'message error';
                msg.textContent = 'Connection error';
            }
            setTimeout(() => msg.className = '', 3000);
        });

        loadCharacters();
    </script>
</body>
</html>`
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleLogout logs the user out.
func (s *AccountServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil {
		s.sessionsMu.Lock()
		delete(s.sessions, cookie.Value)
		s.sessionsMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:   "session",
		Value:  "",
		Path:   "/",
		MaxAge: -1,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleAPIRegister handles JSON API registration.
func (s *AccountServer) handleAPIRegister(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}

	clientIP := s.getClientIP(r)
	err := s.createAccount(clientIP, req.Username, req.Password)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPILogin handles JSON API login.
func (s *AccountServer) handleAPILogin(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}

	account, err := s.accountStore.GetByUsername(req.Username)
	if err != nil {
		account = nil // counted all the same: a guess at a name is a guess
	}
	if ok, refusal := s.checkPassword(r, account, req.Username, req.Password); !ok {
		if refusal == "" {
			refusal = "Invalid username or password"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": refusal})
		return
	}

	s.createSession(w, account.ID, account.Username)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPILogoutSessions ends the account's sessions in the game, given its
// password: a login refused with "You are already logged in." (one login at a
// time, listener.go) offers it, so a player whose other window is gone, hung
// or elsewhere can take the account over. The answer for a wrong password is
// the login page's, so it tells nothing about an account to anyone else.
func (s *AccountServer) handleAPILogoutSessions(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost || s.EndSessions == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}
	account, err := s.accountStore.GetByUsername(req.Username)
	if err != nil {
		account = nil // counted all the same: a guess at a name is a guess
	}
	if ok, refusal := s.checkPassword(r, account, req.Username, req.Password); !ok {
		if refusal == "" {
			refusal = "Invalid username or password"
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": refusal})
		return
	}
	n := s.EndSessions(account.ID)
	log.Printf("AccountServer: '%s' logged out %d session(s) in the game from %s", account.Username, n, s.getClientIP(r))
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "ended": n})
}

// handleAPICharacters returns the user's characters.
func (s *AccountServer) handleAPICharacters(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	sess := s.getSession(r)
	if sess == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Not logged in"})
		return
	}

	chars, err := s.charStore.GetByAccountID(sess.AccountID)
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to load characters"})
		return
	}

	// Convert to simple format for JSON
	charList := make([]map[string]interface{}, len(chars))
	for i, c := range chars {
		charList[i] = map[string]interface{}{
			"id":     c.ServerID,
			"name":   c.CharName,
			"level":  c.Level,
			"race":   c.Race,
			"class":  c.TClass,
			"gender": c.Sex,
			// Where they are, so a teleport has a visible before and after. The
			// stored position is the LAST SAVED one — a character who is logged
			// in right now is somewhere else, and the page says so rather than
			// presenting a stale row as current.
			"world": c.World,
			"x":     c.X,
			"y":     c.Y,
			"z":     c.Z,
		}
	}

	json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "characters": charList})
}

// SetNewCharacterRules sets what a character made on the page starts with,
// as the game's own creation does (database/newchar.go).
func (s *AccountServer) SetNewCharacterRules(r database.NewCharacterRules) {
	s.charStore.SetNewCharacterRules(r)
}

// handleAPICreateCharacter creates a new character.
func (s *AccountServer) handleAPICreateCharacter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	sess := s.getSession(r)
	if sess == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Not logged in"})
		return
	}

	var req struct {
		Name   string `json:"name"`
		Race   int    `json:"race"`
		Class  int    `json:"class"`
		Gender int    `json:"gender"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}

	// Validate name
	req.Name = strings.TrimSpace(req.Name)
	if len(req.Name) < 3 || len(req.Name) > 15 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Name must be 3-15 characters"})
		return
	}
	for _, c := range req.Name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Name can only contain letters"})
			return
		}
	}

	// Validate race/class/gender
	if req.Race < 0 || req.Race > 9 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid race"})
		return
	}
	if req.Class < 0 || req.Class > 14 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid class"})
		return
	}
	if req.Gender < 0 || req.Gender > 1 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid gender"})
		return
	}

	// Check character limit (8 per account)
	chars, _ := s.charStore.GetByAccountID(sess.AccountID)
	if len(chars) >= 8 {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Maximum 8 characters per account"})
		return
	}

	// Check if name exists
	exists, _ := s.charStore.NameExists(req.Name)
	if exists {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Name already taken"})
		return
	}

	// Get model ID for race (male models as default for web API)
	modelIDs := map[int]int64{
		0: 1893243078,  // Human male
		1: -657100808,  // Elf male
		2: 2128125354,  // Dark Elf male
		3: -1449366763, // Gnome male
		4: -1436875705, // Dwarf male
		5: 1282385202,  // Troll male
		6: 1640644319,  // Barbarian male
		7: -1545912350, // Halfling male
		8: 839650119,   // Erudite male
		9: -2071956336, // Ogre male
	}

	record := &database.CharacterRecord{
		AccountID:    sess.AccountID,
		CharName:     req.Name,
		Level:        1,
		Race:         req.Race,
		TClass:       req.Class,
		Sex:          req.Gender,
		ModelID:      modelIDs[req.Race],
		HairColor:    2, // Default appearance values
		HairLength:   2,
		HairStyle:    3,
		FaceOption:   2,
		CurrentHP:    100,
		CurrentPower: 100,
		Breath:       255,
		Speed:        8.0,
	}

	// at the start level, with the class's spells when the server gives them
	spells, err := s.charStore.CreateNew(record)
	if err != nil && record.ServerID == 0 {
		log.Printf("AccountServer: failed to create character: %v", err)
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to create character"})
		return
	}
	if err != nil {
		log.Printf("AccountServer: %v", err)
	}

	log.Printf("AccountServer: character '%s' created for account %d (level %d, %d spells)", req.Name, sess.AccountID, record.Level, spells)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// handleAPIDeleteCharacter deletes a character.
func (s *AccountServer) handleAPIDeleteCharacter(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Method not allowed"})
		return
	}

	sess := s.getSession(r)
	if sess == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Not logged in"})
		return
	}

	var req struct {
		ID int `json:"id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Invalid request"})
		return
	}

	// Verify the character belongs to this account
	char, err := s.charStore.GetByServerID(uint32(req.ID))
	if err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Character not found"})
		return
	}
	if char.AccountID != sess.AccountID {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Not your character"})
		return
	}

	if err := s.charStore.Delete(uint32(req.ID)); err != nil {
		json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "Failed to delete character"})
		return
	}

	log.Printf("AccountServer: character '%s' (ServerID=%d) deleted by account %d", char.CharName, req.ID, sess.AccountID)
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

// createAccount creates a new account with validation.
func (s *AccountServer) createAccount(clientIP, username, password string) error {
	username = strings.TrimSpace(username)
	if len(username) < 3 || len(username) > 16 {
		return fmt.Errorf("username must be 3-16 characters")
	}
	for _, c := range username {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return fmt.Errorf("username can only contain letters, numbers, and underscores")
		}
	}
	if len(password) < 4 {
		return fmt.Errorf("password must be at least 4 characters")
	}

	s.createdIPsMu.RLock()
	alreadyCreated := s.createdIPs[clientIP]
	s.createdIPsMu.RUnlock()
	if alreadyCreated {
		return fmt.Errorf("account already created from this IP address")
	}

	_, err := s.accountStore.GetByUsername(username)
	if err == nil {
		return fmt.Errorf("username already taken")
	}

	hash, err := auth.HashPassword(password, s.bcryptCost)
	if err != nil {
		log.Printf("AccountServer: failed to hash password for '%s': %v", username, err)
		return fmt.Errorf("failed to create account")
	}

	_, err = s.accountStore.Create(username, hash)
	if err != nil {
		log.Printf("AccountServer: failed to create account '%s': %v", username, err)
		return fmt.Errorf("failed to create account")
	}

	s.createdIPsMu.Lock()
	s.createdIPs[clientIP] = true
	s.createdIPsMu.Unlock()

	log.Printf("AccountServer: account '%s' created from IP %s", username, clientIP)
	return nil
}

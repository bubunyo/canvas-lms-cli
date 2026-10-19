package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/bubunyo/canvas-cli/internal/canvas"
	"github.com/bubunyo/canvas-cli/internal/creds"
	"github.com/bubunyo/canvas-cli/internal/webview"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Log in, inspect, or log out"}

var (
	loginToken       string
	loginCookiesOnly bool
	loginExpiryDays  int
	loginTimeout     time.Duration
	loginVerbose     bool
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Log in via SSO (or --token) and store credentials in the keychain",
	Long: `Opens a login window on the Canvas SSO page. After you complete
your institution's sign-in (and MFA), the CLI reads the Canvas session cookie,
uses it to create a personal access token, and stores that token in the OS
keychain. If token creation is not permitted, the session cookies are stored instead.

Use --token to skip the login window and store an access token you created yourself
(Account → Settings → New Access Token). Pass --token - to read it from stdin.`,
	RunE: func(cmd *cobra.Command, _ []string) error {
		ctx := cmd.Context()
		if loginToken != "" {
			return loginWithToken(ctx)
		}
		return loginWithBrowser(ctx)
	},
}

func loginWithToken(ctx context.Context) error {
	tok := loginToken
	if tok == "-" {
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && line == "" {
			return fmt.Errorf("reading token from stdin: %w", err)
		}
		tok = line
	}
	c := &creds.Credentials{Kind: creds.KindToken, Token: strings.TrimSpace(tok), CreatedAt: time.Now()}
	me, err := whoami(ctx, c)
	if err != nil {
		return fmt.Errorf("token rejected: %w", err)
	}
	if err := saveLogin(c); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s (token).\n", cfg.BaseURL, me.Name)
	return nil
}

func loginWithBrowser(ctx context.Context) error {
	fmt.Println("Opening a sign-in window — complete the login there…")
	var logf func(string, ...any)
	if loginVerbose {
		logf = func(f string, args ...any) { fmt.Fprintf(os.Stderr, "  · "+f+"\n", args...) }
	}
	var me *canvas.User
	validate := func(ctx context.Context, s *webview.Session) (bool, error) {
		u, err := whoami(ctx, &creds.Credentials{Kind: creds.KindCookie, Cookies: s.Cookies, CSRF: s.CSRF})
		if errors.Is(err, canvas.ErrUnauthorized) {
			return false, nil
		}
		me = u
		return err == nil, err
	}
	sess, err := webview.Login(ctx, cfg.BaseURL, loginTimeout, validate, logf)
	if err != nil {
		return err
	}
	cookieCreds := &creds.Credentials{Kind: creds.KindCookie, Cookies: sess.Cookies, CSRF: sess.CSRF, CreatedAt: time.Now()}

	if !loginCookiesOnly {
		tc, err := createToken(ctx, cookieCreds)
		if err == nil {
			if err := saveLogin(tc); err != nil {
				return err
			}
			exp := "never expires"
			if !tc.ExpiresAt.IsZero() {
				exp = "expires " + tc.ExpiresAt.Local().Format("02 Jan 2006")
			}
			fmt.Printf("Logged in to %s as %s (access token created, %s).\n", cfg.BaseURL, me.Name, exp)
			return nil
		}
		fmt.Fprintf(os.Stderr, "warning: could not create an access token (%v); falling back to session cookies\n", err)
	}

	if err := saveLogin(cookieCreds); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s (session cookies; re-run `canvas auth login` when it expires).\n", cfg.BaseURL, me.Name)
	return nil
}

func createToken(ctx context.Context, cookieCreds *creds.Credentials) (*creds.Credentials, error) {
	host, _ := os.Hostname()
	body := map[string]any{"purpose": "canvas-cli (" + host + ")"}
	if loginExpiryDays > 0 {
		body["expires_at"] = time.Now().AddDate(0, 0, loginExpiryDays).UTC().Format(time.RFC3339)
	}
	var at canvas.AccessToken
	if err := canvas.New(cfg.BaseURL, cookieCreds).Post(ctx, "/api/v1/users/self/tokens", map[string]any{"token": body}, &at); err != nil {
		return nil, err
	}
	tok := at.Token
	if tok == "" || strings.Contains(tok, "*") {
		tok = at.VisibleToken
	}
	if tok == "" || strings.Contains(tok, "*") {
		return nil, errors.New("server did not return the token value")
	}
	var id int64
	fmt.Sscan(string(at.ID), &id)
	c := &creds.Credentials{Kind: creds.KindToken, Token: tok, TokenID: id, CreatedAt: time.Now()}
	if at.ExpiresAt != nil {
		c.ExpiresAt = *at.ExpiresAt
	}
	return c, nil
}

func whoami(ctx context.Context, c *creds.Credentials) (*canvas.User, error) {
	var me canvas.User
	if err := canvas.New(cfg.BaseURL, c).Get(ctx, "/api/v1/users/self", nil, &me); err != nil {
		return nil, err
	}
	return &me, nil
}

// saveLogin replaces any previous credentials, revoking a token this CLI created earlier.
func saveLogin(c *creds.Credentials) error {
	if old, err := creds.Load(cfg.BaseURL); err == nil && old.TokenID != 0 && old.Token != c.Token {
		_ = canvas.New(cfg.BaseURL, old).Delete(context.Background(), fmt.Sprintf("/api/v1/users/self/tokens/%d", old.TokenID))
	}
	if err := creds.Save(cfg.BaseURL, c); err != nil {
		return fmt.Errorf("saving to keychain: %w", err)
	}
	return cfg.Save()
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show who you are logged in as",
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := creds.Load(cfg.BaseURL)
		if errors.Is(err, creds.ErrNotFound) {
			fmt.Printf("Not logged in to %s.\n", cfg.BaseURL)
			return nil
		}
		if err != nil {
			return err
		}
		me, err := whoami(cmd.Context(), c)
		if err != nil {
			return err
		}
		if jsonOut {
			return printJSON(me)
		}
		fmt.Printf("Instance:  %s\nUser:      %s (id %s", cfg.BaseURL, me.Name, me.ID)
		if me.LoginID != "" {
			fmt.Printf(", login %s", me.LoginID)
		}
		fmt.Printf(")\nMethod:    %s\nSince:     %s\n", c.Kind, c.CreatedAt.Local().Format(time.DateTime))
		if !c.ExpiresAt.IsZero() {
			fmt.Printf("Expires:   %s\n", c.ExpiresAt.Local().Format(time.DateTime))
		}
		return nil
	},
}

var logoutCmd = &cobra.Command{
	Use:   "logout",
	Short: "Revoke the CLI's token and remove stored credentials",
	RunE: func(cmd *cobra.Command, _ []string) error {
		c, err := creds.Load(cfg.BaseURL)
		if err != nil && !errors.Is(err, creds.ErrNotFound) {
			return err
		}
		if c != nil && c.TokenID != 0 {
			if err := canvas.New(cfg.BaseURL, c).Delete(cmd.Context(), fmt.Sprintf("/api/v1/users/self/tokens/%d", c.TokenID)); err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not revoke token on server: %v\n", err)
			}
		}
		if err := creds.Delete(cfg.BaseURL); err != nil {
			return err
		}
		fmt.Printf("Logged out of %s.\n", cfg.BaseURL)
		return nil
	},
}

func init() {
	loginCmd.Flags().StringVar(&loginToken, "token", "", "use an existing access token instead of browser login (\"-\" reads stdin)")
	loginCmd.Flags().BoolVar(&loginCookiesOnly, "cookies-only", false, "store the browser session cookies; don't create an access token")
	loginCmd.Flags().IntVar(&loginExpiryDays, "expires-days", 90, "lifetime of the created access token in days (0 = no expiry)")
	loginCmd.Flags().DurationVar(&loginTimeout, "timeout", 5*time.Minute, "how long to wait for the SSO login to finish")
	loginCmd.Flags().BoolVarP(&loginVerbose, "verbose", "v", false, "log cookie names seen and session checks during login")
	authCmd.AddCommand(loginCmd, statusCmd, logoutCmd)
	rootCmd.AddCommand(authCmd)
}

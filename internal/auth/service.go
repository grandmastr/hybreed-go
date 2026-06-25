package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grandmastr/hybreed-go/internal/config"
	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

const (
	purposeVerifyEmail   = "verify_email"
	purposeResetPassword = "reset_password"

	minPasswordLen = 8
)

// Service implements the authentication and account lifecycle.
type Service struct {
	pool   *pgxpool.Pool
	q      *store.Queries
	tokens *TokenManager
	cfg    config.Config
	log    *slog.Logger
}

// NewService constructs the auth service.
func NewService(pool *pgxpool.Pool, q *store.Queries, tokens *TokenManager, cfg config.Config, log *slog.Logger) *Service {
	return &Service{pool: pool, q: q, tokens: tokens, cfg: cfg, log: log}
}

// ── DTOs ────────────────────────────────────────────────────────────────────

// UserDTO is the client-facing representation of an account.
type UserDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Email         string `json:"email"`
	Handle        string `json:"handle"`
	EmailVerified bool   `json:"emailVerified"`
}

// Session is a freshly minted access + refresh token pair plus the user.
type Session struct {
	AccessToken  string    `json:"accessToken"`
	RefreshToken string    `json:"refreshToken"`
	TokenType    string    `json:"tokenType"`
	ExpiresAt    time.Time `json:"expiresAt"`
	User         UserDTO   `json:"user"`
}

// RegisterResult is returned after sign-up; the client then collects the OTP.
type RegisterResult struct {
	User                 UserDTO `json:"user"`
	VerificationRequired bool    `json:"verificationRequired"`
}

func toUserDTO(u store.User) UserDTO {
	return UserDTO{
		ID:            u.ID.String(),
		Name:          u.Name,
		Email:         u.Email,
		Handle:        u.Handle,
		EmailVerified: u.EmailVerified,
	}
}

// ── Operations ──────────────────────────────────────────────────────────────

// Register creates an unverified account and issues an email-verification OTP.
func (s *Service) Register(ctx context.Context, name, email, password string) (RegisterResult, error) {
	name = strings.TrimSpace(name)
	email, err := normalizeEmail(email)
	if err != nil {
		return RegisterResult{}, err
	}
	if name == "" {
		return RegisterResult{}, httpx.ErrValidation("name is required")
	}
	if len(password) < minPasswordLen {
		return RegisterResult{}, httpx.ErrValidation(fmt.Sprintf("password must be at least %d characters", minPasswordLen))
	}

	if _, err := s.q.GetUserByEmail(ctx, email); err == nil {
		return RegisterResult{}, httpx.ErrConflict("an account with this email already exists")
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return RegisterResult{}, fmt.Errorf("lookup user: %w", err)
	}

	hash, err := HashPassword(password)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("hash password: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // no-op after a successful commit

	qtx := s.q.WithTx(tx)
	user, err := qtx.CreateUser(ctx, store.CreateUserParams{
		Name:          name,
		Email:         email,
		PasswordHash:  store.Ptr(hash),
		EmailVerified: false,
	})
	if err != nil {
		return RegisterResult{}, fmt.Errorf("create user: %w", err)
	}
	if _, err := qtx.CreateUserSettings(ctx, user.ID); err != nil {
		return RegisterResult{}, fmt.Errorf("create settings: %w", err)
	}
	code, err := s.issueOTP(ctx, qtx, user.ID, purposeVerifyEmail)
	if err != nil {
		return RegisterResult{}, fmt.Errorf("issue otp: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return RegisterResult{}, fmt.Errorf("commit tx: %w", err)
	}

	s.deliverOTP(user.Email, code)
	return RegisterResult{User: toUserDTO(user), VerificationRequired: true}, nil
}

// VerifyEmail consumes the OTP, marks the account verified, and starts a session.
func (s *Service) VerifyEmail(ctx context.Context, email, code, userAgent string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, err
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, httpx.ErrNotFound("no account for this email")
		}
		return Session{}, fmt.Errorf("lookup user: %w", err)
	}

	if err := s.consumeOTP(ctx, user.ID, purposeVerifyEmail, code); err != nil {
		return Session{}, err
	}
	if err := s.q.SetEmailVerified(ctx, user.ID); err != nil {
		return Session{}, fmt.Errorf("set verified: %w", err)
	}
	user.EmailVerified = true
	return s.issueSession(ctx, user, userAgent)
}

// Login authenticates with email + password. Unverified accounts get a fresh OTP
// and a 403 so the client can route to the verify screen.
func (s *Service) Login(ctx context.Context, email, password, userAgent string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, err
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, httpx.ErrUnauthorized("invalid email or password")
		}
		return Session{}, fmt.Errorf("lookup user: %w", err)
	}
	if user.PasswordHash == nil || !CheckPassword(*user.PasswordHash, password) {
		return Session{}, httpx.ErrUnauthorized("invalid email or password")
	}
	if !user.EmailVerified {
		if code, err := s.issueOTP(ctx, s.q, user.ID, purposeVerifyEmail); err == nil {
			s.deliverOTP(user.Email, code)
		}
		return Session{}, httpx.NewError(403, "email_not_verified", "verify your email to continue")
	}
	return s.issueSession(ctx, user, userAgent)
}

// Refresh rotates a refresh token: the old one is revoked and a new pair issued.
func (s *Service) Refresh(ctx context.Context, refreshToken, userAgent string) (Session, error) {
	if refreshToken == "" {
		return Session{}, httpx.ErrUnauthorized("refresh token is required")
	}
	sess, err := s.q.GetRefreshSession(ctx, sha256hex(refreshToken))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, httpx.ErrUnauthorized("invalid or expired refresh token")
		}
		return Session{}, fmt.Errorf("lookup session: %w", err)
	}
	if err := s.q.RevokeRefreshSession(ctx, sha256hex(refreshToken)); err != nil {
		return Session{}, fmt.Errorf("revoke session: %w", err)
	}
	user, err := s.q.GetUserByID(ctx, sess.UserID)
	if err != nil {
		return Session{}, fmt.Errorf("lookup user: %w", err)
	}
	return s.issueSession(ctx, user, userAgent)
}

// Logout revokes the supplied refresh token (idempotent).
func (s *Service) Logout(ctx context.Context, refreshToken string) error {
	if refreshToken == "" {
		return nil
	}
	if err := s.q.RevokeRefreshSession(ctx, sha256hex(refreshToken)); err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

// ResendOTP issues a fresh verification code for an unverified account.
func (s *Service) ResendOTP(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrNotFound("no account for this email")
		}
		return fmt.Errorf("lookup user: %w", err)
	}
	if user.EmailVerified {
		return httpx.ErrBadRequest("email is already verified")
	}
	code, err := s.issueOTP(ctx, s.q, user.ID, purposeVerifyEmail)
	if err != nil {
		return fmt.Errorf("issue otp: %w", err)
	}
	s.deliverOTP(user.Email, code)
	return nil
}

// RequestPasswordReset issues a password-reset OTP for the account. To avoid
// leaking which emails are registered, it always reports success — the code is
// only issued and delivered when an account actually exists.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	email, err := normalizeEmail(email)
	if err != nil {
		return err
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // don't reveal whether the email is registered
		}
		return fmt.Errorf("lookup user: %w", err)
	}
	code, err := s.issueOTP(ctx, s.q, user.ID, purposeResetPassword)
	if err != nil {
		return fmt.Errorf("issue otp: %w", err)
	}
	s.deliverOTP(user.Email, code)
	return nil
}

// ResetPassword consumes the reset OTP, sets a new password, and starts a session.
// Completing a reset also verifies the email (the user proved address control).
func (s *Service) ResetPassword(ctx context.Context, email, code, password, userAgent string) (Session, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, err
	}
	if len(password) < minPasswordLen {
		return Session{}, httpx.ErrValidation(fmt.Sprintf("password must be at least %d characters", minPasswordLen))
	}
	user, err := s.q.GetUserByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Session{}, httpx.ErrBadRequest("no pending reset; request a new code")
		}
		return Session{}, fmt.Errorf("lookup user: %w", err)
	}
	if err := s.consumeOTP(ctx, user.ID, purposeResetPassword, code); err != nil {
		return Session{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Session{}, fmt.Errorf("hash password: %w", err)
	}
	if err := s.q.UpdateUserPassword(ctx, store.UpdateUserPasswordParams{ID: user.ID, PasswordHash: store.Ptr(hash)}); err != nil {
		return Session{}, fmt.Errorf("update password: %w", err)
	}
	if !user.EmailVerified {
		if err := s.q.SetEmailVerified(ctx, user.ID); err != nil {
			return Session{}, fmt.Errorf("set verified: %w", err)
		}
		user.EmailVerified = true
	}
	return s.issueSession(ctx, user, userAgent)
}

// SocialInput carries a stubbed social-login payload.
//
// TODO: verify the provider ID token (Apple/Google) server-side before trusting
// the email. For now we accept the client-supplied identity for development.
type SocialInput struct {
	Provider string
	Email    string
	Name     string
}

// Social signs in (or provisions) an account from a social provider. STUBBED.
func (s *Service) Social(ctx context.Context, in SocialInput, userAgent string) (Session, error) {
	switch in.Provider {
	case "apple", "google":
	default:
		return Session{}, httpx.ErrBadRequest("unsupported provider")
	}
	email := in.Email
	if email == "" {
		email = fmt.Sprintf("demo.%s@hybreed.app", in.Provider)
	}
	email, err := normalizeEmail(email)
	if err != nil {
		return Session{}, err
	}

	user, err := s.q.GetUserByEmail(ctx, email)
	if errors.Is(err, pgx.ErrNoRows) {
		name := in.Name
		if name == "" {
			name = "Alex Carter"
		}
		user, err = s.q.CreateUser(ctx, store.CreateUserParams{
			Name:          name,
			Email:         email,
			PasswordHash:  nil, // social-only account
			EmailVerified: true,
		})
		if err != nil {
			return Session{}, fmt.Errorf("create social user: %w", err)
		}
		if _, err := s.q.CreateUserSettings(ctx, user.ID); err != nil {
			return Session{}, fmt.Errorf("create settings: %w", err)
		}
	} else if err != nil {
		return Session{}, fmt.Errorf("lookup user: %w", err)
	}
	return s.issueSession(ctx, user, userAgent)
}

// Me returns the current account.
func (s *Service) Me(ctx context.Context, userID uuid.UUID) (UserDTO, error) {
	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return UserDTO{}, httpx.ErrNotFound("account not found")
		}
		return UserDTO{}, fmt.Errorf("lookup user: %w", err)
	}
	return toUserDTO(user), nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

func (s *Service) issueSession(ctx context.Context, user store.User, userAgent string) (Session, error) {
	access, expiresAt, err := s.tokens.Generate(user.ID, user.Email)
	if err != nil {
		return Session{}, fmt.Errorf("generate access token: %w", err)
	}
	refresh, err := newOpaqueToken()
	if err != nil {
		return Session{}, fmt.Errorf("generate refresh token: %w", err)
	}
	if _, err := s.q.CreateRefreshSession(ctx, store.CreateRefreshSessionParams{
		UserID:    user.ID,
		TokenHash: sha256hex(refresh),
		UserAgent: userAgent,
		ExpiresAt: store.TS(time.Now().Add(s.cfg.RefreshTokenTTL)),
	}); err != nil {
		return Session{}, fmt.Errorf("persist refresh session: %w", err)
	}
	return Session{
		AccessToken:  access,
		RefreshToken: refresh,
		TokenType:    "Bearer",
		ExpiresAt:    expiresAt,
		User:         toUserDTO(user),
	}, nil
}

func (s *Service) issueOTP(ctx context.Context, q store.Querier, userID uuid.UUID, purpose string) (string, error) {
	if err := q.InvalidateUserOTPs(ctx, store.InvalidateUserOTPsParams{UserID: userID, Purpose: purpose}); err != nil {
		return "", err
	}
	code, err := generateOTP()
	if err != nil {
		return "", err
	}
	if _, err := q.CreateOTP(ctx, store.CreateOTPParams{
		UserID:    userID,
		CodeHash:  sha256hex(code),
		Purpose:   purpose,
		ExpiresAt: store.TS(time.Now().Add(s.cfg.OTPTTL)),
	}); err != nil {
		return "", err
	}
	return code, nil
}

func (s *Service) consumeOTP(ctx context.Context, userID uuid.UUID, purpose, code string) error {
	otp, err := s.q.GetActiveOTP(ctx, store.GetActiveOTPParams{UserID: userID, Purpose: purpose})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return httpx.ErrBadRequest("no pending code; request a new one")
		}
		return fmt.Errorf("lookup otp: %w", err)
	}
	if store.TimeOf(otp.ExpiresAt).Before(time.Now()) {
		return httpx.ErrBadRequest("code has expired; request a new one")
	}
	if int(otp.Attempts) >= s.cfg.OTPMaxAttempts {
		return httpx.ErrBadRequest("too many attempts; request a new code")
	}
	if otp.CodeHash != sha256hex(code) {
		_ = s.q.IncrementOTPAttempts(ctx, otp.ID)
		return httpx.ErrUnauthorized("incorrect code")
	}
	if err := s.q.ConsumeOTP(ctx, otp.ID); err != nil {
		return fmt.Errorf("consume otp: %w", err)
	}
	return nil
}

// deliverOTP "sends" the code. In development we log it; production should wire a
// real email/SMS provider here.
func (s *Service) deliverOTP(email, code string) {
	if s.cfg.IsProduction() {
		s.log.Info("otp issued", "email", email)
		// TODO: send `code` via an email/SMS provider.
		return
	}
	s.log.Info("otp issued (dev)", "email", email, "code", code)
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if _, err := mail.ParseAddress(email); err != nil {
		return "", httpx.ErrValidation("a valid email is required")
	}
	return email, nil
}

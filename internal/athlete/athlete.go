// Package athlete serves the "You" tab: profile, settings, PRs and stats.
package athlete

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/grandmastr/hybreed-go/internal/httpx"
	"github.com/grandmastr/hybreed-go/internal/store"
)

// Service implements the athlete-profile use cases.
type Service struct {
	q   *store.Queries
	log *slog.Logger
}

// NewService builds the athlete service.
func NewService(q *store.Queries, log *slog.Logger) *Service { return &Service{q: q, log: log} }

// ── DTOs ────────────────────────────────────────────────────────────────────

type ProfileDTO struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	Handle       string    `json:"handle"`
	Status       string    `json:"status"`
	Streak       int32     `json:"streak"`
	LoadTarget   int32     `json:"loadTarget"`
	BodyWeightKg *float64  `json:"bodyWeightKg,omitempty"`
	MemberSince  time.Time `json:"memberSince"`
}

type SettingsDTO struct {
	Units         string   `json:"units"`
	Notifications bool     `json:"notifications"`
	ConnectedApps int32    `json:"connectedApps"`
	BodyWeightKg  *float64 `json:"bodyWeightKg,omitempty"`
}

type PRDTO struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Value string `json:"value"`
	Icon  string `json:"icon"`
}

type MonthTotals struct {
	DistanceKm     float64 `json:"distanceKm"`
	VolumeKg       int64   `json:"volumeKg"`
	VolumeT        float64 `json:"volumeT"`
	Sessions       int64   `json:"sessions"`
	CaloriesLogged int64   `json:"caloriesLogged"`
}

type WeekPoint struct {
	Week string `json:"week"` // ISO date of the week's Monday
	Load int64  `json:"load"`
}

type StatsDTO struct {
	Month           MonthTotals `json:"month"`
	WeeklyLoadTrend []WeekPoint `json:"weeklyLoadTrend"`
}

// ── Operations ──────────────────────────────────────────────────────────────

func numPtr(n pgtype.Numeric) *float64 {
	if !n.Valid {
		return nil
	}
	v := store.Float(n)
	return &v
}

// GetProfile returns the athlete's identity card.
func (s *Service) GetProfile(ctx context.Context, userID uuid.UUID) (ProfileDTO, error) {
	user, err := s.q.GetUserByID(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ProfileDTO{}, httpx.ErrNotFound("account not found")
		}
		return ProfileDTO{}, fmt.Errorf("get user: %w", err)
	}
	settings, err := s.ensureSettings(ctx, userID)
	if err != nil {
		return ProfileDTO{}, err
	}
	return ProfileDTO{
		ID:           user.ID.String(),
		Name:         user.Name,
		Email:        user.Email,
		Handle:       user.Handle,
		Status:       user.Status,
		Streak:       user.Streak,
		LoadTarget:   user.LoadTarget,
		BodyWeightKg: numPtr(settings.BodyWeightKg),
		MemberSince:  store.TimeOf(user.CreatedAt),
	}, nil
}

// UpdateProfile applies a partial update to the athlete's identity.
func (s *Service) UpdateProfile(ctx context.Context, userID uuid.UUID, p store.UpdateUserProfileParams) (ProfileDTO, error) {
	p.ID = userID
	if _, err := s.q.UpdateUserProfile(ctx, p); err != nil {
		return ProfileDTO{}, fmt.Errorf("update profile: %w", err)
	}
	return s.GetProfile(ctx, userID)
}

// GetSettings returns the athlete's settings (creating defaults if absent).
func (s *Service) GetSettings(ctx context.Context, userID uuid.UUID) (SettingsDTO, error) {
	settings, err := s.ensureSettings(ctx, userID)
	if err != nil {
		return SettingsDTO{}, err
	}
	return toSettingsDTO(settings), nil
}

// UpdateSettings applies a partial update to settings.
func (s *Service) UpdateSettings(ctx context.Context, userID uuid.UUID, p store.UpdateUserSettingsParams) (SettingsDTO, error) {
	if _, err := s.ensureSettings(ctx, userID); err != nil {
		return SettingsDTO{}, err
	}
	p.UserID = userID
	updated, err := s.q.UpdateUserSettings(ctx, p)
	if err != nil {
		return SettingsDTO{}, fmt.Errorf("update settings: %w", err)
	}
	return toSettingsDTO(updated), nil
}

// ListPRs returns the athlete's personal records.
func (s *Service) ListPRs(ctx context.Context, userID uuid.UUID) ([]PRDTO, error) {
	rows, err := s.q.ListPersonalRecords(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("list prs: %w", err)
	}
	out := make([]PRDTO, 0, len(rows))
	for _, r := range rows {
		out = append(out, PRDTO{ID: r.ID.String(), Label: r.Label, Value: r.Value, Icon: r.Icon})
	}
	return out, nil
}

// CreatePR adds a personal record.
func (s *Service) CreatePR(ctx context.Context, p store.CreatePersonalRecordParams) (PRDTO, error) {
	r, err := s.q.CreatePersonalRecord(ctx, p)
	if err != nil {
		return PRDTO{}, fmt.Errorf("create pr: %w", err)
	}
	return PRDTO{ID: r.ID.String(), Label: r.Label, Value: r.Value, Icon: r.Icon}, nil
}

// DeletePR removes a personal record owned by the user.
func (s *Service) DeletePR(ctx context.Context, userID, id uuid.UUID) error {
	if err := s.q.DeletePersonalRecord(ctx, store.DeletePersonalRecordParams{ID: id, UserID: userID}); err != nil {
		return fmt.Errorf("delete pr: %w", err)
	}
	return nil
}

// GetStats returns month-to-date totals and a 6-week load trend.
func (s *Service) GetStats(ctx context.Context, userID uuid.UUID) (StatsDTO, error) {
	now := time.Now().UTC()
	monthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthStop := monthStart.AddDate(0, 1, 0)

	distance, err := s.q.MonthlyDistance(ctx, store.MonthlyDistanceParams{UserID: userID, Start: store.TS(monthStart), Stop: store.TS(monthStop)})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("monthly distance: %w", err)
	}
	volume, err := s.q.MonthlyVolume(ctx, store.MonthlyVolumeParams{UserID: userID, Start: store.TS(monthStart), Stop: store.TS(monthStop)})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("monthly volume: %w", err)
	}
	sessions, err := s.q.CountSessionsInRange(ctx, store.CountSessionsInRangeParams{UserID: userID, Start: store.TS(monthStart), Stop: store.TS(monthStop)})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("monthly sessions: %w", err)
	}
	calories, err := s.q.SumCaloriesInRange(ctx, store.SumCaloriesInRangeParams{UserID: userID, Start: store.DateOf(monthStart), Stop: store.DateOf(monthStop)})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("monthly calories: %w", err)
	}

	trendStart := mondayOf(now).AddDate(0, 0, -7*5)
	rows, err := s.q.WeeklyLoadTrend(ctx, store.WeeklyLoadTrendParams{UserID: userID, Start: store.TS(trendStart)})
	if err != nil {
		return StatsDTO{}, fmt.Errorf("weekly trend: %w", err)
	}

	return StatsDTO{
		Month: MonthTotals{
			DistanceKm:     float64(distance) / 1000,
			VolumeKg:       volume,
			VolumeT:        float64(volume) / 1000,
			Sessions:       sessions,
			CaloriesLogged: calories,
		},
		WeeklyLoadTrend: buildTrend(rows, now),
	}, nil
}

func (s *Service) ensureSettings(ctx context.Context, userID uuid.UUID) (store.UserSetting, error) {
	settings, err := s.q.GetUserSettings(ctx, userID)
	if err == nil {
		return settings, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return store.UserSetting{}, fmt.Errorf("get settings: %w", err)
	}
	created, err := s.q.CreateUserSettings(ctx, userID)
	if err != nil {
		return store.UserSetting{}, fmt.Errorf("create settings: %w", err)
	}
	return created, nil
}

func toSettingsDTO(s store.UserSetting) SettingsDTO {
	return SettingsDTO{
		Units:         s.Units,
		Notifications: s.Notifications,
		ConnectedApps: s.ConnectedApps,
		BodyWeightKg:  numPtr(s.BodyWeightKg),
	}
}

// mondayOf returns 00:00 UTC of the Monday in t's week.
func mondayOf(t time.Time) time.Time {
	t = t.UTC()
	offset := (int(t.Weekday()) + 6) % 7 // days since Monday (Sun=0 → 6)
	d := t.AddDate(0, 0, -offset)
	return time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
}

// buildTrend fills 6 consecutive weeks (oldest→newest) from sparse query rows.
func buildTrend(rows []store.WeeklyLoadTrendRow, now time.Time) []WeekPoint {
	loads := make(map[string]int64, len(rows))
	for _, r := range rows {
		loads[store.DateValue(r.Week).Format("2006-01-02")] = r.Load
	}
	monday := mondayOf(now)
	out := make([]WeekPoint, 0, 6)
	for i := 5; i >= 0; i-- {
		key := monday.AddDate(0, 0, -7*i).Format("2006-01-02")
		out = append(out, WeekPoint{Week: key, Load: loads[key]})
	}
	return out
}

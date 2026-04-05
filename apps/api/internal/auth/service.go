package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"hyperspeed/api/internal/store"
)

type Service struct {
	Store  *store.Store
	secret []byte
}

func NewService(st *store.Store, jwtSecret string) *Service {
	return &Service{Store: st, secret: []byte(jwtSecret)}
}

func HashPassword(plain string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	return string(b), err
}

func CheckPassword(hash, plain string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}

func NewRefreshToken() (raw string, hash string, err error) {
	var b [32]byte
	if _, err = rand.Read(b[:]); err != nil {
		return "", "", err
	}
	raw = hex.EncodeToString(b[:])
	h := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(h[:])
	return raw, hash, nil
}

const refreshTTL = 7 * 24 * time.Hour

func (s *Service) IssueTokens(ctx context.Context, userID uuid.UUID, email string) (access, refresh string, err error) {
	access, err = s.SignAccess(userID, email)
	if err != nil {
		return "", "", err
	}
	raw, hash, err := NewRefreshToken()
	if err != nil {
		return "", "", err
	}
	exp := time.Now().Add(refreshTTL)
	if err := s.Store.SaveRefreshToken(ctx, userID, hash, exp); err != nil {
		return "", "", err
	}
	return access, raw, nil
}

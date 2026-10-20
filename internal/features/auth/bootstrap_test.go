package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

const bootstrapPassword = "correct-horse-battery" // 21 bytes, inside the 12–72 range

type createCall struct {
	name, email, passwordHash string
}

// fakeCreator records calls and returns a scripted result.
type fakeCreator struct {
	calls   []createCall
	code    int64
	created bool
	err     error
}

func (f *fakeCreator) CreateIfAbsent(_ context.Context, name, email, passwordHash string) (int64, bool, error) {
	f.calls = append(f.calls, createCall{name, email, passwordHash})
	return f.code, f.created, f.err
}

func TestBootstrapAdminCreatesUserWithBcryptHash(t *testing.T) {
	creator := &fakeCreator{code: 7, created: true}

	result, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{
		Email:    "  ops@workshop.example  ",
		Password: bootstrapPassword,
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if !result.Created || result.Code != 7 {
		t.Fatalf("result = %+v, want Created with code 7", result)
	}
	if len(creator.calls) != 1 {
		t.Fatalf("CreateIfAbsent called %d times, want 1", len(creator.calls))
	}
	call := creator.calls[0]
	if call.email != "ops@workshop.example" {
		t.Errorf("email = %q, want it trimmed", call.email)
	}
	if call.name != defaultAdminName {
		t.Errorf("name = %q, want default %q", call.name, defaultAdminName)
	}
	if call.passwordHash == bootstrapPassword {
		t.Fatal("the plaintext password was passed to the repository")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(call.passwordHash), []byte(bootstrapPassword)); err != nil {
		t.Errorf("stored hash does not verify against the password: %v", err)
	}
	if cost, err := bcrypt.Cost([]byte(call.passwordHash)); err != nil || cost != bcrypt.DefaultCost {
		t.Errorf("bcrypt cost = %d (err %v), want %d", cost, err, bcrypt.DefaultCost)
	}
}

func TestBootstrapAdminKeepsGivenName(t *testing.T) {
	creator := &fakeCreator{created: true, code: 1}
	if _, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{
		Name: " Demo Admin ", Email: "demo@workshop.example", Password: bootstrapPassword,
	}); err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if got := creator.calls[0].name; got != "Demo Admin" {
		t.Errorf("name = %q, want %q", got, "Demo Admin")
	}
}

func TestBootstrapAdminExistingUserIsNotCreated(t *testing.T) {
	creator := &fakeCreator{created: false}
	result, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{
		Email: "ops@workshop.example", Password: bootstrapPassword,
	})
	if err != nil {
		t.Fatalf("BootstrapAdmin: %v", err)
	}
	if result.Created || result.Code != 0 {
		t.Errorf("result = %+v, want not created", result)
	}
}

func TestBootstrapAdminRejectsInvalidEmail(t *testing.T) {
	for _, email := range []string{
		"",
		"   ",
		"not-an-address",
		"Ops <ops@workshop.example>",
		"<ops@workshop.example>",
		"ops@workshop.example, other@workshop.example",
	} {
		creator := &fakeCreator{created: true}
		_, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{Email: email, Password: bootstrapPassword})
		if !errors.Is(err, ErrInvalidAdminEmail) {
			t.Errorf("email %q: err = %v, want ErrInvalidAdminEmail", email, err)
		}
		if len(creator.calls) != 0 {
			t.Errorf("email %q: repository was called for invalid input", email)
		}
	}
}

func TestBootstrapAdminRejectsPasswordOutsideBounds(t *testing.T) {
	for _, password := range []string{
		"",
		strings.Repeat("a", minAdminPasswordLength-1),
		strings.Repeat("a", maxAdminPasswordLength+1),
	} {
		creator := &fakeCreator{created: true}
		_, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{Email: "ops@workshop.example", Password: password})
		if !errors.Is(err, ErrInvalidAdminPassword) {
			t.Errorf("password of %d bytes: err = %v, want ErrInvalidAdminPassword", len(password), err)
		}
		if len(creator.calls) != 0 {
			t.Errorf("password of %d bytes: repository was called for invalid input", len(password))
		}
	}
}

func TestBootstrapAdminAcceptsPasswordBounds(t *testing.T) {
	for _, length := range []int{minAdminPasswordLength, maxAdminPasswordLength} {
		creator := &fakeCreator{created: true, code: 1}
		if _, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{
			Email: "ops@workshop.example", Password: strings.Repeat("p", length),
		}); err != nil {
			t.Errorf("password of %d bytes: unexpected error %v", length, err)
		}
	}
}

func TestBootstrapAdminPropagatesRepositoryErrorWithoutSecrets(t *testing.T) {
	creator := &fakeCreator{err: errors.New(`relation "users" does not exist`)}
	_, err := BootstrapAdmin(context.Background(), creator, AdminBootstrapInput{
		Email: "ops@workshop.example", Password: bootstrapPassword,
	})
	if err == nil || !errors.Is(err, creator.err) {
		t.Fatalf("err = %v, want it to wrap the repository error", err)
	}
	if strings.Contains(err.Error(), bootstrapPassword) || strings.Contains(err.Error(), "$2a$") {
		t.Errorf("error message leaks the password or its hash: %q", err)
	}
}

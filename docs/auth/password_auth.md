# Password Authentication

This document describes the password authentication system, including the login flow, temporary passwords, password change requirements, and security considerations.

## Overview

Password authentication requires users to enter a password to complete login. This method is selected per-user via the `auth_method` field set to `"password"`.

**Key features:**
- Bcrypt password hashing (cost 12)
- Temporary password support (forces change on first login)
- Common password blocking
- Minimum/maximum length requirements

## Authentication Flow

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PASSWORD AUTHENTICATION FLOW                         │
└─────────────────────────────────────────────────────────────────────────────┘

User                    Browser                      Server
 │                         │                            │
 │  Enter login ID         │                            │
 │ ───────────────────────>│                            │
 │                         │  POST /login               │
 │                         │ ──────────────────────────>│
 │                         │                            │  Look up user
 │                         │                            │  Check auth_method = "password"
 │                         │  Redirect to               │
 │                         │  /login/password           │
 │                         │ <──────────────────────────│
 │                         │                            │
 │  Enter password         │                            │
 │ ───────────────────────>│                            │
 │                         │  POST /login/password      │
 │                         │ ──────────────────────────>│
 │                         │                            │  Verify password (bcrypt)
 │                         │                            │
 │                         │         [If password_temp = true]
 │                         │                            │
 │                         │  Redirect to               │
 │                         │  /login/change-password    │
 │                         │ <──────────────────────────│
 │                         │                            │
 │  Enter new password     │                            │
 │  Confirm password       │                            │
 │ ───────────────────────>│                            │
 │                         │  POST /login/change-password
 │                         │ ──────────────────────────>│
 │                         │                            │  Validate password
 │                         │                            │  Hash and save
 │                         │                            │  Set password_temp = false
 │                         │                            │
 │                         │         [Normal login completion]
 │                         │                            │
 │                         │                            │  Create session
 │                         │  Redirect to /dashboard    │
 │                         │ <──────────────────────────│
 │                         │                            │
```

## Routes

Defined in `internal/app/features/login/routes.go`:

| Method | Path | Handler | Purpose |
|--------|------|---------|---------|
| GET | `/login` | ServeLogin | Display login ID form |
| POST | `/login` | HandleLoginPost | Process login ID, start password flow |
| GET | `/login/password` | ServePasswordPage | Display password entry form |
| POST | `/login/password` | HandlePasswordSubmit | Verify password |
| GET | `/login/change-password` | ServeChangePassword | Display change password form |
| POST | `/login/change-password` | HandleChangePassword | Validate and save new password |

## Handler Functions

Located in `internal/app/features/login/handler.go`:

### `startPasswordFlow()`
Initiates password authentication:
1. Stores pending login state in session (`pending_user_id`, `pending_login_id`)
2. Clears any existing authentication
3. Redirects to `/login/password`

### `HandlePasswordSubmit()`
Processes password entry:
1. Retrieves pending user from session
2. Loads user from database
3. Verifies password using bcrypt
4. If `password_temp` is true, redirects to change password
5. Otherwise, creates session and redirects to dashboard

### `HandleChangePassword()`
Processes password change:
1. Validates new password matches confirmation
2. Validates password meets requirements
3. Hashes new password with bcrypt
4. Updates database (sets `password_hash`, clears `password_temp`)
5. Creates session and redirects to dashboard

## Password Storage

### User Model Fields

```go
type User struct {
    // ... other fields ...
    PasswordHash *string `bson:"password_hash"` // bcrypt hash
    PasswordTemp *bool   `bson:"password_temp"` // true if temporary
}
```

### Hashing

Passwords are hashed using bcrypt with cost factor 12:

```go
const BcryptCost = 12

func HashPassword(password string) (string, error) {
    hash, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
    if err != nil {
        return "", err
    }
    return string(hash), nil
}

func CheckPassword(password, hash string) bool {
    err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
    return err == nil
}
```

## Temporary Passwords

Administrators can set temporary passwords when creating or editing users. Temporary passwords must be changed on first login.

### How It Works

1. Admin sets password with `password_temp = true`
2. User enters login ID and password
3. Password is verified successfully
4. System detects `password_temp = true`
5. User is redirected to change password form
6. User enters and confirms new password
7. New password is saved with `password_temp = false`
8. User is logged in

### Admin Interface

When editing a user with password auth method:
- Leave password field blank to keep existing password
- Enter a new password to reset it (marked as temporary)
- User will be required to change on next login

## Password Validation

Located in `internal/app/system/authutil/password.go`:

### Requirements

| Rule | Value |
|------|-------|
| Minimum length | 6 characters |
| Maximum length | 128 characters |
| Common passwords | Blocked |

### Common Password List

The following passwords are blocked (case-insensitive):

```
123456, 1234567, 12345678, 123456789, password, password1,
qwerty, qwerty123, abc123, abcdef, 111111, 000000, 123123,
654321, iloveyou, monkey, dragon, master, letmein, welcome,
login, admin, princess, sunshine, football, baseball, soccer,
hockey, batman, superman
```

### Validation Function

```go
func ValidatePassword(password string) error {
    if len(password) < MinPasswordLength {
        return ErrPasswordTooShort
    }
    if len(password) > MaxPasswordLength {
        return ErrPasswordTooLong
    }
    if commonPasswords[strings.ToLower(password)] {
        return ErrPasswordCommon
    }
    return nil
}
```

### User-Facing Rules

Displayed on change password form:

> Password must be at least 6 characters and cannot be a common password like "123456" or "password".

## Session Management

### Pending State (during authentication)

```go
sess.Values["pending_user_id"]    = userID.Hex()
sess.Values["pending_login_id"]   = loginID
sess.Values["pending_return_url"] = returnURL
```

### Authenticated State (after successful login)

```go
sess.Values["is_authenticated"] = true
sess.Values["user_id"]          = userID.Hex()
// pending_* values are cleared
```

## Templates

| File | Purpose |
|------|---------|
| `login/templates/password.gohtml` | Password entry form |
| `login/templates/change_password.gohtml` | Change password form (for temp passwords) |

### Password Form

Shows:
- Login ID (read-only, with "Not you?" link)
- Password field
- Login button

### Change Password Form

Shows:
- Warning about temporary password
- Login ID (read-only)
- Password rules
- New password field
- Confirm password field
- Change Password button

## Error Messages

| Scenario | Message |
|----------|---------|
| No password set | "No password set for this account. Please contact an administrator." |
| Wrong password | "Incorrect password. Please try again." |
| Password too short | "Password must be at least 6 characters." |
| Password too long | "Password must be less than 128 characters." |
| Common password | "This password is too common. Please choose a different one." |
| Passwords don't match | "Passwords do not match." |

## Security Considerations

### Bcrypt Cost Factor

Cost factor 12 provides a good balance between security and performance:
- ~250ms to hash on modern hardware
- Resistant to brute-force attacks
- Adjustable if hardware improves

### Common Password Protection

Blocking common passwords prevents:
- Dictionary attacks
- Easy-to-guess passwords
- Passwords that appear in breach databases

### Temporary Password Flow

Forcing password change ensures:
- Users choose their own passwords
- Admin-set passwords aren't permanent
- Initial passwords can be shared securely (e.g., in person)

### No Rate Limiting (Current)

Password authentication does not currently implement rate limiting. Consider adding per-user attempt limits for production deployments to prevent brute-force attacks.

## Configuration

### Constants

In `internal/app/system/authutil/password.go`:

```go
const (
    MinPasswordLength = 6
    MaxPasswordLength = 128
    BcryptCost        = 12
)
```

### Adding to Common Password List

Edit the `commonPasswords` map in `password.go`:

```go
var commonPasswords = map[string]bool{
    "newpassword": true,
    // ... existing passwords
}
```

## File Reference

| File | Purpose |
|------|---------|
| `internal/app/features/login/handler.go` | Login flow handlers |
| `internal/app/features/login/routes.go` | Route definitions |
| `internal/app/features/login/templates/password.gohtml` | Password entry UI |
| `internal/app/features/login/templates/change_password.gohtml` | Change password UI |
| `internal/app/system/authutil/password.go` | Password hashing and validation |

## See Also

- [Email Auth](email_auth.md) - Email verification authentication
- [Trust Auth](trust_auth.md) - Trust authentication
- [Auth Plan](auth_plan.md) - Overall authentication architecture
- [Configuration](../configuration.md) - Full configuration reference

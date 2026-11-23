package auth

import "context"

// Role is what a caller may do in Ember. Roles are ordered: an admin may do
// everything a user may.
type Role int

const (
	// RoleNone means no role was established for the request.
	RoleNone Role = iota
	// RoleUser may read Ember, use projects, notes and chats, and call plugin operations.
	RoleUser
	// RoleAdmin may also configure, activate and deactivate plugins.
	RoleAdmin
)

func (r Role) String() string {
	switch r {
	case RoleUser:
		return "user"
	case RoleAdmin:
		return "admin"
	}
	return "none"
}

type roleKey struct{}

// WithRole returns a context carrying the caller's role. Wrap sets it on every
// request it passes to the API.
func WithRole(ctx context.Context, role Role) context.Context {
	return context.WithValue(ctx, roleKey{}, role)
}

// RoleFrom returns the caller's role, or RoleNone when Wrap did not set one.
func RoleFrom(ctx context.Context) Role {
	role, _ := ctx.Value(roleKey{}).(Role)
	return role
}

// roleFor maps the ID token's groups to a role at sign-in. Admin groups come
// first and admit their members even when allowedGroups does not list them.
func (a *Authenticator) roleFor(user User) Role {
	switch {
	case memberOf(user.Groups, a.cfg.AdminGroups):
		return RoleAdmin
	case len(a.cfg.AllowedGroups) == 0, memberOf(user.Groups, a.cfg.AllowedGroups):
		return RoleUser
	}
	return RoleNone
}

func memberOf(groups, wanted []string) bool {
	for _, group := range groups {
		for _, w := range wanted {
			if group == w {
				return true
			}
		}
	}
	return false
}

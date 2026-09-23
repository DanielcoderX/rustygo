package rustygo

import (
	"context"
	"net/http"
)

type contextKey struct{}

var arenaContextKey = contextKey{}

// WithContextArena attaches an Arena to a context.
func WithContextArena(ctx context.Context, a *Arena) context.Context {
	return context.WithValue(ctx, arenaContextKey, a)
}

// FromContext extracts an Arena from context.
func FromContext(ctx context.Context) (*Arena, bool) {
	if ctx == nil {
		return nil, false
	}
	a, ok := ctx.Value(arenaContextKey).(*Arena)
	return a, ok
}

type scopeContextKey struct{}

var scopeKey = scopeContextKey{}

// WithContextScope attaches an active Scope to context and returns a cleanup function that exits the scope.
func WithContextScope(ctx context.Context, s *Scope) (context.Context, func()) {
	ctx = context.WithValue(ctx, scopeKey, s)
	cleanup := func() {
		if s != nil && s.Active() {
			s.Exit()
		}
	}
	return ctx, cleanup
}

// ScopeFromContext extracts the active Scope from context.
func ScopeFromContext(ctx context.Context) (*Scope, bool) {
	if ctx == nil {
		return nil, false
	}
	s, ok := ctx.Value(scopeKey).(*Scope)
	return s, ok
}

// HTTPMiddleware creates an HTTP middleware that binds an entered Scope to every request context
// and automatically exits the scope when the HTTP request finishes.
func HTTPMiddleware(a *Arena) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s := a.EnterScope()
			defer func() {
				if s.Active() {
					s.Exit()
				}
			}()
			ctx, _ := WithContextScope(r.Context(), s)
			ctx = WithContextArena(ctx, a)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

package httpapi

import (
	"context"

	"github.com/heridotlife/Setagaya/internal/domain/account"
)

// ctxKey is the private type for request-context keys.
type ctxKey int

const accountKey ctxKey = iota

// withAccount returns a context carrying the authenticated account.
func withAccount(ctx context.Context, acct account.Account) context.Context {
	return context.WithValue(ctx, accountKey, acct)
}

// accountFrom returns the authenticated account stored by the auth middleware,
// or the anonymous (zero) account when none is present.
func accountFrom(ctx context.Context) account.Account {
	acct, _ := ctx.Value(accountKey).(account.Account)
	return acct
}

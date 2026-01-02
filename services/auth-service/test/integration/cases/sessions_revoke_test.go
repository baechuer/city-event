//go:build integration

package cases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	itinfra "github.com/baechuer/real-time-ressys/services/auth-service/test/integration/infra"
)

func Test_SessionsRevoke_InvalidatesRefresh(t *testing.T) {
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	d := MustNewDeps(t, env)
	defer d.Close(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	require.NoError(t, itinfra.ResetAll(ctx, d.DB, d.RDB))

	reg, err := d.Svc.Register(ctx, "it_revoke@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)

	login, err := d.Svc.Login(ctx, "it_revoke@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)

	require.NoError(t, d.Svc.SessionsRevoke(ctx, reg.User.ID))

	_, _, err = d.Svc.Refresh(ctx, login.Tokens.RefreshToken)
	require.Error(t, err)
}

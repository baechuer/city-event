//go:build integration

package cases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	itinfra "github.com/baechuer/real-time-ressys/services/auth-service/test/integration/infra"
)

func Test_Login_Then_Refresh_Rotates(t *testing.T) {
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	d := MustNewDeps(t, env)
	defer d.Close(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	require.NoError(t, itinfra.ResetAll(ctx, d.DB, d.RDB))

	_, err = d.Svc.Register(ctx, "it_login@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)

	login, err := d.Svc.Login(ctx, "it_login@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)
	require.NotEmpty(t, login.Tokens.RefreshToken)

	rotated, _, err := d.Svc.Refresh(ctx, login.Tokens.RefreshToken)
	require.NoError(t, err)

	// rotated 是 auth.AuthTokens（不是带 Tokens 字段的 struct）
	require.NotEmpty(t, rotated.RefreshToken)
	require.NotEqual(t, login.Tokens.RefreshToken, rotated.RefreshToken)

	// 老 refresh 应该失效（rotate delete old）
	_, _, err = d.Svc.Refresh(ctx, login.Tokens.RefreshToken)
	require.Error(t, err)
}

//go:build integration

package cases

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	itinfra "github.com/baechuer/real-time-ressys/services/auth-service/test/integration/infra"
)

func Test_PasswordChange_RevokesSessions(t *testing.T) {
	env, err := itinfra.LoadEnv()
	require.NoError(t, err)

	d := MustNewDeps(t, env)
	defer d.Close(t)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	require.NoError(t, itinfra.ResetAll(ctx, d.DB, d.RDB))

	reg, err := d.Svc.Register(ctx, "it_pw@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)

	login, err := d.Svc.Login(ctx, "it_pw@example.com", "StrongPassw0rd!!")
	require.NoError(t, err)

	ok := callPasswordChangeIfExists(d.Svc, ctx, reg.User.ID, "StrongPassw0rd!!", "NewStrongPassw0rd!!")
	if !ok {
		t.Skip("auth.Service has no password-change method with a supported signature (try implementing PasswordChange/ChangePassword/UpdatePassword)")
	}

	// refresh token 应该被 revoke（若你实现里 password change 会触发 revokeAll/token version bump）
	_, _, err = d.Svc.Refresh(ctx, login.Tokens.RefreshToken)
	require.Error(t, err)

	// 新密码应该能登录
	_, err = d.Svc.Login(ctx, "it_pw@example.com", "NewStrongPassw0rd!!")
	require.NoError(t, err)
}

// 尝试匹配你 service 的真实方法名（不匹配就返回 false）
func callPasswordChangeIfExists(svc any, ctx context.Context, userID, oldPw, newPw string) bool {
	type v1 interface {
		PasswordChange(context.Context, string, string, string) error
	}
	if x, ok := svc.(v1); ok {
		_ = x.PasswordChange(ctx, userID, oldPw, newPw)
		return true
	}

	type v2 interface {
		ChangePassword(context.Context, string, string, string) error
	}
	if x, ok := svc.(v2); ok {
		_ = x.ChangePassword(ctx, userID, oldPw, newPw)
		return true
	}

	type v3 interface {
		UpdatePassword(context.Context, string, string, string) error
	}
	if x, ok := svc.(v3); ok {
		_ = x.UpdatePassword(ctx, userID, oldPw, newPw)
		return true
	}

	return false
}

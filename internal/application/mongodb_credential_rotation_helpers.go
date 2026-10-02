package application

import (
	"context"
	"errors"
	"strings"

	bhruntime "github.com/mcpdev80/baseharbor/internal/runtime"
)

func mongoDBUpsertUser(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, authUser, authPassword, database, username, password, role string) error {
	script := "IFS= read -r auth_user\nIFS= read -r auth_password\nIFS= read -r database\nIFS= read -r username\nIFS= read -r password\nIFS= read -r role\nexport ROTATE_DB=\"$database\" ROTATE_USER=\"$username\" ROTATE_PASSWORD=\"$password\" ROTATE_ROLE=\"$role\"\nmongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username \"$auth_user\" --password \"$auth_password\" --authenticationDatabase admin --eval 'const d=db.getSiblingDB(process.env.ROTATE_DB); const u=process.env.ROTATE_USER; const p=process.env.ROTATE_PASSWORD; const r=process.env.ROTATE_ROLE; if (d.getUser(u)) { d.updateUser(u,{pwd:p,roles:[{role:r,db:process.env.ROTATE_DB}]}); } else { d.createUser({user:u,pwd:p,roles:[{role:r,db:process.env.ROTATE_DB}]}); }'\n"
	input := []byte(authUser + "\n" + authPassword + "\n" + database + "\n" + username + "\n" + password + "\n" + role + "\n")
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, input, service, "sh", "-ceu", script)
	return err
}

func mongoDBDropUser(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, authUser, authPassword, database, username string) error {
	script := "IFS= read -r auth_user\nIFS= read -r auth_password\nIFS= read -r database\nIFS= read -r username\nexport ROTATE_DB=\"$database\" ROTATE_USER=\"$username\"\nmongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username \"$auth_user\" --password \"$auth_password\" --authenticationDatabase admin --eval 'const d=db.getSiblingDB(process.env.ROTATE_DB); if (d.getUser(process.env.ROTATE_USER)) d.dropUser(process.env.ROTATE_USER);'\n"
	_, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(authUser+"\n"+authPassword+"\n"+database+"\n"+username+"\n"), service, "sh", "-ceu", script)
	return err
}

func mongoDBVerifyCredential(ctx context.Context, runtime bhruntime.RuntimeProvider, files RuntimeFiles, service, username, password, database string, wantAccepted bool) error {
	script := "IFS= read -r username\nIFS= read -r password\nIFS= read -r database\nif mongosh --quiet --host localhost --tls --tlsCAFile /run/baseharbor/tls/ca.pem --username \"$username\" --password \"$password\" --authenticationDatabase \"$database\" --eval 'db.adminCommand({ping:1}).ok' >/dev/null 2>&1; then printf accepted; else printf rejected; fi\n"
	out, err := runtime.ExecProjectInput(ctx, files.Project, files.Compose, files.Env, []byte(username+"\n"+password+"\n"+database+"\n"), service, "sh", "-ceu", script)
	if err != nil {
		return err
	}
	accepted := strings.TrimSpace(out) == "accepted"
	if accepted != wantAccepted {
		if wantAccepted {
			return errors.New("MongoDB credential was rejected")
		}
		return errors.New("retired MongoDB credential is still accepted")
	}
	return nil
}

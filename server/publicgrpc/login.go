package publicgrpc

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"errors"

	"github.com/navidrome/navidrome/conf"
	"github.com/navidrome/navidrome/core/auth"
	"github.com/navidrome/navidrome/log"
	"github.com/navidrome/navidrome/model"
	"github.com/navidrome/navidrome/server/publicgrpc/gen"
	"github.com/navidrome/navidrome/utils/gravatar"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *Service) Login(ctx context.Context, req *gen.LoginRequest) (*gen.LoginResponse, error) {
	if !conf.PublicGRPCEnabled() {
		return nil, status.Error(codes.Unavailable, "public gRPC is disabled")
	}
	user, err := validateCredentials(ctx, s.ds, req.GetUsername(), req.GetPassword())
	if err != nil {
		return nil, err
	}
	return loginResponse(user)
}

func (s *Service) CreateAdmin(ctx context.Context, req *gen.CreateAdminRequest) (*gen.LoginResponse, error) {
	if !conf.PublicGRPCEnabled() {
		return nil, status.Error(codes.Unavailable, "public gRPC is disabled")
	}
	if s.ds == nil {
		return nil, status.Error(codes.Unavailable, "datastore unavailable")
	}
	username := req.GetUsername()
	password := req.GetPassword()
	if username == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "username and password are required")
	}
	var user *model.User
	err := s.ds.WithTxImmediate(func(tx model.DataStore) error {
		count, err := tx.User(ctx).CountAll()
		if err != nil {
			return err
		}
		if count > 0 {
			return errAdminExists
		}
		user = &model.User{
			UserName:    username,
			Name:        username,
			NewPassword: password,
			IsAdmin:     true,
		}
		return tx.User(ctx).Put(user)
	}, "grpc create admin")
	if errors.Is(err, errAdminExists) {
		return nil, status.Error(codes.FailedPrecondition, "initial admin already exists")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create admin: %v", err)
	}
	user, err = s.ds.User(ctx).FindByUsername(username)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load admin: %v", err)
	}
	return loginResponse(user)
}

var errAdminExists = errors.New("admin already exists")

func validateCredentials(ctx context.Context, ds model.DataStore, username, password string) (*model.User, error) {
	if ds == nil {
		return nil, status.Error(codes.Unavailable, "datastore unavailable")
	}
	if username == "" || password == "" {
		return nil, status.Error(codes.InvalidArgument, "username and password are required")
	}
	u, err := ds.User(ctx).FindByUsernameWithPassword(username)
	if errors.Is(err, model.ErrNotFound) {
		return nil, status.Error(codes.Unauthenticated, "invalid username or password")
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "authenticate: %v", err)
	}
	if u.Password != password {
		log.Warn(ctx, "Unsuccessful gRPC login", "username", username)
		return nil, status.Error(codes.Unauthenticated, "invalid username or password")
	}
	if err := ds.User(ctx).UpdateLastLoginAt(u.ID); err != nil {
		log.Error(ctx, "Could not update LastLoginAt", "user", username, err)
	}
	return u, nil
}

func loginResponse(user *model.User) (*gen.LoginResponse, error) {
	token, err := auth.CreateToken(user)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "create token: %v", err)
	}
	resp := &gen.LoginResponse{
		Token:    token,
		Id:       user.ID,
		Name:     user.Name,
		Username: user.UserName,
		IsAdmin:  user.IsAdmin,
	}
	if conf.Server.EnableGravatar && user.Email != "" {
		resp.Avatar = gravatar.Url(user.Email, 50)
	}
	salt := make([]byte, 3)
	if _, err := rand.Read(salt); err == nil {
		resp.SubsonicSalt = hex.EncodeToString(salt)
		sum := md5.Sum([]byte(user.Password + resp.SubsonicSalt))
		resp.SubsonicToken = hex.EncodeToString(sum[:])
	}
	return resp, nil
}

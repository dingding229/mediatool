package embyapi

import (
	"encoding/json"
	"errors"
	"io"
	"media-tool/internal/modules/embyapi/dto"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/platform/httpjson"
	"net/http"
	"strings"
	"time"
)

func deviceInfo(info RequestContext) contracts.DeviceInfo {
	return contracts.DeviceInfo{DeviceID: info.Auth.DeviceID, Name: info.Auth.Device, Client: info.Auth.Client, Version: info.Auth.Version, UserAgent: info.UserAgent, ClientIP: info.Address.ClientIP}
}
func authUserDTO(u contracts.User, serverID string) dto.UserDto {
	limit := u.MaxStreams
	if limit < 1 {
		limit = 2
	}
	return dto.UserDto{Id: u.ID, Name: u.Name, ServerId: serverID, HasPassword: true, HasConfiguredPassword: true, Policy: dto.UserPolicy{IsAdministrator: false, IsDisabled: !u.Enabled, EnableMediaPlayback: u.CanPlay, EnableContentDownloading: u.CanDownload, EnabledFolders: dto.Array[string]{}, SimultaneousStreamLimit: &limit}, Configuration: dto.UserConfiguration{SubtitleMode: "Default"}}
}
func (s *Server) authLogin(w http.ResponseWriter, r *http.Request, info RequestContext, value nativelibrary.ServerSettings) {
	var in struct {
		Username string `json:"Username"`
		Pw       string `json:"Pw"`
		Password string `json:"Password"`
	}
	if !decodeAuthBody(w, r, &in) {
		return
	}
	if in.Pw != "" && in.Password != "" && in.Pw != in.Password {
		writeAuthError(w, 400, "密码字段冲突")
		return
	}
	password := in.Pw
	if password == "" {
		password = in.Password
	}
	if s.options.Auth == nil {
		writeAuthError(w, 503, "媒体认证服务不可用")
		return
	}
	result, err := s.options.Auth.Login(r.Context(), in.Username, password, deviceInfo(info))
	if err != nil {
		switch {
		case errors.Is(err, contracts.ErrPasswordChangeRequired):
			writeAuthError(w, 401, "该媒体用户需要管理员在后台设置最终密码后才能使用 Emby 登录")
		case errors.Is(err, contracts.ErrRateLimited):
			w.Header().Set("Retry-After", "60")
			writeAuthError(w, 429, "登录尝试过于频繁，请稍后重试")
		case errors.Is(err, contracts.ErrUnavailable):
			writeAuthError(w, 503, "媒体认证服务暂不可用")
		default:
			writeAuthError(w, 401, "账号或密码不正确")
		}
		return
	}
	session := &dto.SessionInfo{Id: result.Device.ID, UserId: result.User.ID, UserName: result.User.Name, DeviceId: result.Device.DeviceKey, DeviceName: info.Auth.Device, Client: info.Auth.Client, ApplicationVersion: info.Auth.Version, LastActivityDate: utcNow()}
	s.write(w, dto.AuthenticationResult{User: authUserDTO(result.User, value.ServerID), SessionInfo: session, AccessToken: result.AccessToken, ServerId: value.ServerID})
}
func (s *Server) authUser(w http.ResponseWriter, r *http.Request, info RequestContext, value nativelibrary.ServerSettings) {
	if s.options.Auth == nil {
		s.router.fail(w, 503)
		return
	}
	if canonicalUserID(r.PathValue("userId")) != info.User.ID {
		s.router.fail(w, 403)
		return
	}
	user, err := s.options.Auth.GetUser(r.Context(), info.User.ID)
	if err != nil {
		if errors.Is(err, contracts.ErrUnavailable) {
			s.router.fail(w, 503)
		} else {
			s.router.fail(w, 401)
		}
		return
	}
	s.write(w, authUserDTO(user, value.ServerID))
}
func (s *Server) authPassword(w http.ResponseWriter, r *http.Request, info RequestContext) {
	if canonicalUserID(r.PathValue("userId")) != info.User.ID {
		s.router.fail(w, 403)
		return
	}
	var in struct {
		CurrentPw string `json:"CurrentPw"`
		NewPw     string `json:"NewPw"`
		Password  string `json:"Password"`
	}
	if !decodeAuthBody(w, r, &in) {
		return
	}
	if in.NewPw != "" && in.Password != "" && in.NewPw != in.Password {
		writeAuthError(w, 400, "密码字段冲突")
		return
	}
	next := in.NewPw
	if next == "" {
		next = in.Password
	}
	if s.options.Auth == nil {
		writeAuthError(w, 503, "媒体认证服务不可用")
		return
	}
	if err := s.options.Auth.ChangePassword(r.Context(), *info.User, *info.Token, in.CurrentPw, next); err != nil {
		switch {
		case errors.Is(err, contracts.ErrRateLimited):
			w.Header().Set("Retry-After", "60")
			writeAuthError(w, 429, "修改请求过于频繁，请稍后重试")
		case errors.Is(err, contracts.ErrInvalidPassword):
			writeAuthError(w, 400, "新密码不符合密码规则")
		case errors.Is(err, contracts.ErrUnauthenticated):
			writeAuthError(w, 401, "当前密码不正确")
		default:
			writeAuthError(w, 503, "密码修改暂不可用")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) authLogout(w http.ResponseWriter, r *http.Request, info RequestContext) {
	if s.options.Auth == nil {
		writeAuthError(w, 503, "媒体认证服务不可用")
		return
	}
	if err := s.options.Auth.Logout(r.Context(), *info.Token); err != nil {
		if errors.Is(err, contracts.ErrUnavailable) {
			s.router.fail(w, 503)
		} else {
			s.router.fail(w, 401)
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (s *Server) authPublicUsers(w http.ResponseWriter, r *http.Request, value nativelibrary.ServerSettings) {
	if !value.PublicUserList {
		s.write(w, []struct {
			Name, Id    string
			HasPassword bool
		}{})
		return
	}
	if s.options.Auth == nil {
		s.router.fail(w, 503)
		return
	}
	users, err := s.options.Auth.PublicUsers(r.Context())
	if err != nil {
		s.router.fail(w, 503)
		return
	}
	s.write(w, users)
}
func utcNow() *dto.UTCDateTime { v := dto.NewUTCDateTime(time.Now().UTC()); return &v }
func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// All messages are fixed protocol strings, never wrapped database errors.
	if err := httpjson.Write(w, status, struct {
		ResponseStatus struct{ ErrorCode, Message string }
	}{struct{ ErrorCode, Message string }{strings.ReplaceAll(http.StatusText(status), " ", ""), message}}); err != nil {
		return
	}
}

func canonicalUserID(raw string) string { return strings.ReplaceAll(strings.ToLower(raw), "-", "") }

func decodeAuthBody(w http.ResponseWriter, r *http.Request, out any) bool {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10))
	if err := decoder.Decode(out); err != nil {
		writeAuthError(w, 400, "请求格式无效")
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		writeAuthError(w, 400, "只允许一个 JSON 对象")
		return false
	}
	return true
}

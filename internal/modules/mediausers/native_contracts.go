package mediausers

import "media-tool/internal/modules/mediausers/contracts"

// Protocol consumers import contracts, not this database-backed package.
type TokenAuthenticator = contracts.TokenAuthenticator
type PermissionChecker = contracts.PermissionChecker

func NewTokenAuthenticator() TokenAuthenticator { return contracts.NewTokenAuthenticator() }
func NewPermissionChecker() PermissionChecker   { return contracts.NewPermissionChecker() }

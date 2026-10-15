package maintenance

import (
	"crypto/sha256"
	"golang.org/x/sys/unix"
	hashpkg "hash"
	"media-tool/internal/modules/jobs"
)

type hashWriter = hashpkg.Hash

func newSHA() hashpkg.Hash { return sha256.New() }
func noFollowFlag() int    { return unix.O_NOFOLLOW }
func newID() string        { return jobs.UUID() }

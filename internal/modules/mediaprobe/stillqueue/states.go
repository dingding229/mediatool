package stillqueue

type AttemptReason string

const (
	LeaseExpired       AttemptReason = "lease_expired"
	ProcessFailed      AttemptReason = "process_failed"
	InvalidImage       AttemptReason = "invalid_image"
	TMDBStill          AttemptReason = "tmdb_still"
	SourceUnavailable  AttemptReason = "source_unavailable"
	StorageUnavailable AttemptReason = "storage_unavailable"
	StorageFull        AttemptReason = "storage_full"
	CacheEvicted       AttemptReason = "cache_evicted"
)

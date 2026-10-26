package embyapi

import (
	"errors"
	"media-tool/internal/modules/mediausers/contracts"
	"media-tool/internal/modules/nativelibrary"
	"media-tool/internal/modules/nativeplayback"
	"net/http"
	"slices"
)

// Services holds contracts only. The composition root supplies implementations;
// the default skeleton fails closed rather than granting playback/authentication.
type Services struct {
	Libraries     nativelibrary.LibraryService
	Items         nativelibrary.ItemService
	UserData      nativelibrary.UserDataService
	Playback      nativeplayback.StreamResolver
	Authenticator contracts.TokenAuthenticator
	Permissions   contracts.PermissionChecker
}

func NewServices() Services {
	return Services{nativelibrary.NewLibraryService(), nativelibrary.NewItemService(), nativelibrary.NewUserDataService(), nativeplayback.NewStreamResolver(), contracts.NewTokenAuthenticator(), contracts.NewPermissionChecker()}
}

// ItemQuery preserves every T04 parsed field and copies mutable containers.
func (q Query) ItemQuery() nativelibrary.ItemQuery {
	clone := func(v *bool) *bool {
		if v == nil {
			return nil
		}
		value := *v
		return &value
	}
	return nativelibrary.ItemQuery{StartIndex: q.StartIndex, Limit: q.Limit,
		SortBy: slices.Clone(q.SortBy), SortOrder: slices.Clone(q.SortOrder), Fields: slices.Clone(q.Fields), IDs: slices.Clone(q.IDs), IncludeItemTypes: slices.Clone(q.IncludeItemTypes), ExcludeItemTypes: slices.Clone(q.ExcludeItemTypes), ParentID: q.ParentID, SeriesID: q.SeriesID, SeasonID: q.SeasonID, Genres: slices.Clone(q.Genres), Years: slices.Clone(q.Years), SearchTerm: q.SearchTerm,
		Recursive: clone(q.Recursive), IsFolder: clone(q.IsFolder), IsPlayed: clone(q.IsPlayed), IsFavorite: clone(q.IsFavorite), IsResumable: clone(q.IsResumable)}
}

// DomainErrorStatus never exposes the underlying error text. T07+ handlers pass
// this status to WriteError and handle the response write error.
func DomainErrorStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, ErrInvalid), errors.Is(err, nativelibrary.ErrInvalidQuery):
		return http.StatusBadRequest
	case errors.Is(err, contracts.ErrUnauthenticated):
		return http.StatusUnauthorized
	case errors.Is(err, nativeplayback.ErrIPDenied), errors.Is(err, nativeplayback.ErrForbidden), errors.Is(err, nativeplayback.ErrUADenied), errors.Is(err, nativeplayback.ErrParentalDenied):
		return http.StatusForbidden
	case errors.Is(err, nativeplayback.ErrStreamLimit):
		return http.StatusForbidden
	case errors.Is(err, contracts.ErrForbidden), errors.Is(err, contracts.ErrParentalDenied), errors.Is(err, contracts.ErrStreamLimit):
		return http.StatusNotFound
	case errors.Is(err, nativeplayback.ErrNotFound), errors.Is(err, nativelibrary.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, nativeplayback.ErrUpstream), errors.Is(err, contracts.ErrUnavailable), errors.Is(err, nativelibrary.ErrSettingsUnavailable):
		return http.StatusServiceUnavailable
	case errors.Is(err, nativeplayback.ErrNotImplemented), errors.Is(err, nativelibrary.ErrNotImplemented), errors.Is(err, contracts.ErrNotImplemented):
		return http.StatusNotImplemented
	default:
		return http.StatusInternalServerError
	}
}

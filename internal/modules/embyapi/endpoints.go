package embyapi

import "net/http"

type responseKind uint8

const (
	singleResponse responseKind = iota
	listResponse
	writeResponse
	publicInfoResponse
	systemInfoResponse
	endpointResponse
	brandingResponse
	brandingCSSResponse
	displayResponse
	localizationResponse
	socketResponse
	startupSessionsResponse
	userConfigurationResponse
	pingResponse
	publicUsersResponse
	noContentResponse
	imageResponse
	authenticateResponse
	userResponse
	passwordResponse
	logoutResponse
	viewsResponse
	browseResponse
	detailResponse
	countsResponse
	seasonsResponse
	episodesResponse
	latestResponse
	hintsResponse
	genresResponse
	filtersResponse
	similarResponse
	personalResponse
	resumeResponse
	nextUpResponse
	playbackResponse
	reportResponse
	subtitleResponse
	streamResponse
)

type endpoint struct {
	method, pattern string
	kind            responseKind
	public          bool
}

// Keep in sync with the media-port table in docs/NATIVE_EMBY_SERVER.md.
// Administrative /api/v1/media-server routes belong ONLY to the management port.
func nativeEndpoints() []endpoint {
	return []endpoint{
		{http.MethodGet, "/System/Info/Public", publicInfoResponse, true},
		{http.MethodGet, "/System/Ping", pingResponse, true}, {http.MethodPost, "/System/Ping", pingResponse, true},
		{http.MethodGet, "/System/Info", systemInfoResponse, false},
		{http.MethodGet, "/System/Endpoint", endpointResponse, true},
		{http.MethodGet, "/Branding/Configuration", brandingResponse, true},
		{http.MethodGet, "/Branding/Css", brandingCSSResponse, true},
		{http.MethodGet, "/Branding/Css.css", brandingCSSResponse, true},
		{http.MethodGet, "/DisplayPreferences", displayResponse, false},
		{http.MethodGet, "/DisplayPreferences/{id}", displayResponse, false},
		{http.MethodGet, "/Users/{userId}/Configuration", userConfigurationResponse, false},
		{http.MethodGet, "/Sessions", startupSessionsResponse, false},
		{http.MethodGet, "/Localization/*", localizationResponse, false},
		{http.MethodGet, "/socket", socketResponse, false},
		{http.MethodGet, "/Users/Public", publicUsersResponse, true},
		{http.MethodPost, "/Users/AuthenticateByName", authenticateResponse, true},
		{http.MethodGet, "/Users/{userId}", userResponse, false},
		{http.MethodPost, "/Users/{userId}/Password", passwordResponse, false},
		{http.MethodPost, "/Sessions/Logout", logoutResponse, false},
		{http.MethodPost, "/Sessions/Capabilities", noContentResponse, false},
		{http.MethodPost, "/Sessions/Capabilities/Full", noContentResponse, false},
		{http.MethodGet, "/Users/{userId}/Views", viewsResponse, false},
		{http.MethodGet, "/Library/MediaFolders", viewsResponse, false},
		{http.MethodGet, "/Users/{userId}/Items", browseResponse, false},
		{http.MethodGet, "/Users/{userId}/Items/{itemId}", detailResponse, false},
		{http.MethodGet, "/Items/Latest", latestResponse, false},
		{http.MethodGet, "/Users/{userId}/Items/Latest", latestResponse, false},
		{http.MethodGet, "/Items", browseResponse, false},
		{http.MethodGet, "/Items/{id}", detailResponse, false},
		{http.MethodGet, "/Items/Counts", countsResponse, false},
		{http.MethodGet, "/Items/{id}/Similar", similarResponse, false},
		{http.MethodGet, "/Shows/{id}/Seasons", seasonsResponse, false},
		{http.MethodGet, "/Shows/{id}/Episodes", episodesResponse, false},
		{http.MethodGet, "/Shows/NextUp", nextUpResponse, false},
		{http.MethodGet, "/Search/Hints", hintsResponse, false},
		{http.MethodGet, "/Genres", genresResponse, false},
		{http.MethodGet, "/Items/Filters", filtersResponse, false},
		{http.MethodGet, "/Items/{id}/Images/{type}", imageResponse, false},
		{http.MethodGet, "/Items/{id}/Images/{type}/{index}", imageResponse, false},
		{http.MethodGet, "/Items/{id}/PlaybackInfo", playbackResponse, false},
		{http.MethodPost, "/Items/{id}/PlaybackInfo", playbackResponse, false},
		{http.MethodGet, "/Videos/{id}/stream", streamResponse, false},
		{http.MethodGet, "/Videos/{id}/stream.{ext}", streamResponse, false},
		{http.MethodGet, "/Videos/{id}/original.{ext}", streamResponse, false},
		{http.MethodGet, "/Items/{id}/Download", streamResponse, false},
		{http.MethodGet, "/Videos/{id}/subtitles/{index}/stream", subtitleResponse, false},
		{http.MethodGet, "/Videos/{id}/{mediaSourceId}/Subtitles/{index}/Stream.{format}", subtitleResponse, false},
		{http.MethodPost, "/Sessions/Playing", reportResponse, false},
		{http.MethodPost, "/Sessions/Playing/Ping", reportResponse, false},
		{http.MethodPost, "/Users/{userId}/PlayingItems/{itemId}", reportResponse, false},
		{http.MethodDelete, "/Users/{userId}/PlayingItems/{itemId}", reportResponse, false},
		{http.MethodPost, "/Users/{userId}/PlayingItems/{itemId}/Progress", reportResponse, false},
		{http.MethodPost, "/Sessions/Playing/Progress", reportResponse, false},
		{http.MethodPost, "/Sessions/Playing/Stopped", reportResponse, false},
		{http.MethodPost, "/Users/{userId}/PlayedItems/{itemId}", personalResponse, false},
		{http.MethodDelete, "/Users/{userId}/PlayedItems/{itemId}", personalResponse, false},
		{http.MethodPost, "/Users/{userId}/FavoriteItems/{itemId}", personalResponse, false},
		{http.MethodDelete, "/Users/{userId}/FavoriteItems/{itemId}", personalResponse, false},
		{http.MethodGet, "/Users/{userId}/Items/{itemId}/UserData", personalResponse, false},
		{http.MethodPost, "/Users/{userId}/Items/{itemId}/UserData", personalResponse, false},
		{http.MethodGet, "/Users/{userId}/Items/Resume", resumeResponse, false},
	}
}

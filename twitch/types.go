package twitch

import "fmt"

type PlaybackCommand int

const (
	PLAY PlaybackCommand = iota
	RESUME
	PAUSE
	PREVIOUS
	NEXT
	STOP
)

// StreamsResponse container around the Twitch streams response.
type StreamsResponse struct {
	Data []*Stream
	*Pagination
}

// Stream describes the properties for a particular stream on Twitch
type Stream struct {
	ID           string   `json:"id"`
	UserID       string   `json:"user_id"`
	CommunityIDs []string `json:"community_ids"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	ViewerCount  int      `json:"viewer_count"`
	ThumbnailURL string   `json:"thumbnail_url"`
}

func (s *Stream) String() string {
	return fmt.Sprintf("%+v", *s)
}

// UserResponse is a container around the response from the Twitch /users endpoint
type UserResponse struct {
	Data []*User
}

// User contains all the properties for a particular Twitch user.
type User struct {
	ID              string `json:"id"`
	Login           string `json:"login"`
	DisplayName     string `json:"display_name"`
	Type            string `json:"type"`
	BroadcasterType string `json:"broadcaster_type"`
}

func (u *User) String() string {
	return fmt.Sprintf("%+v", *u)
}

// Follows is a wrapper around the response when requesting a set of follower relationships
type Follows struct {
	Data []*Follow
}

// FollowIDsList will extract the user IDs from the calling Follows struct into a single slice
func (follows *Follows) FollowIDsList() []string {

	result := make([]string, 0, len(follows.Data))
	for _, f := range follows.Data {
		result = append(result, f.ToID)
	}

	return result
}

// Follow describes a single follower relationship, FromID is the user doing the following
// and ToID is the user being followed.
type Follow struct {
	FromID string `json:"from_id"`
	ToID   string `json:"to_id"`
}

func (f *Follow) String() string {
	return fmt.Sprintf("%+v", *f)
}

// Pagination wraps the cursor used to perform pagination on endpoints that support it. The
// cursor should be used in following requests to indicate the current page.
type Pagination struct {
	Cursor string `json:"cursor"`
}

// ChannelAccessToken is used for loading the stream URL for a specific channel. For some reason
// this type of request auth needs to be used instead of the other oauth process.
type ChannelAccessToken struct {
	Sig   string `json:"sig"`
	Token string `json:"token"`
}

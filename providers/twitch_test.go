package providers

import (
	"fmt"
	"math/rand"
	"testing"

	"github.com/garyburd/redigo/redis"
)

func setup() {
	InitEnv("redis://127.0.0.1:6379")
}

func teardown() {
	clearRedisLists()
}

func TestSaveCurrentStreamWithNilDoesNothing(t *testing.T) {

	setup()
	defer teardown()

	SaveUsersCurrentStream(nil, createRandomMockStream())
	conn := redisConnPool.Get()
	defer conn.Close()
	reply, _ := redis.Strings(conn.Do("KEYS", "*"))
	if len(reply) != 0 {
		t.Fatalf("Size of recent streams list increased when it souldn't have")
	}

	SaveUsersCurrentStream(createRandomMockUser(), nil)
	reply, _ = redis.Strings(conn.Do("KEYS", "*"))
	if len(reply) != 0 {
		t.Fatalf("Size of recent streams list increased when it shouldn't have")
	}
}

func TestSaveCurrentStreamAppliesExpireTime(t *testing.T) {
	setup()
	defer teardown()

	mockUser := createRandomMockUser()
	listName := fmt.Sprintf("twitch_recent_streams:%s", mockUser.ID)
	mockStream := createRandomMockStream()

	SaveUsersCurrentStream(mockUser, mockStream)
	if !validateListSize(listName, 1) {
		t.Fatalf("Failed to save the mock stream to recents list")
	}

	conn := redisConnPool.Get()
	defer conn.Close()
	ttl, err := redis.Int(conn.Do("TTL", listName))
	if err != nil {
		fmt.Printf("Error trying to load TTL: %s", err.Error())
	}

	if ttl <= 0 {
		t.Fatalf("Bad return value from TTL command on recent stream list: %d\n", ttl)
	}
}

func TestSaveCurrentStream(t *testing.T) {
	setup()
	defer teardown()

	mockUser1 := createRandomMockUser()
	listName1 := fmt.Sprintf("twitch_recent_streams:%s", mockUser1.ID)
	mockUser2 := createRandomMockUser()
	listName2 := fmt.Sprintf("twitch_recent_streams:%s", mockUser2.ID)

	mockStream1 := createRandomMockStream()
	mockStream2 := createRandomMockStream()

	SaveUsersCurrentStream(mockUser1, mockStream1)
	if !validateListSize(listName1, 1) ||
		!validateListSize(listName2, 0) {
		t.Fatalf("Failed to save current stream")
	}

	SaveUsersCurrentStream(mockUser2, mockStream2)
	if !validateListSize(listName2, 1) ||
		!validateListSize(listName1, 1) {
		t.Fatalf("Failed to save second recent stream")
	}

	SaveUsersCurrentStream(mockUser1, mockStream2)
	if !validateListSize(listName1, 2) ||
		!validateListSize(listName2, 1) {
		t.Fatalf("Inserted recent stream into the wrong user's list")
	}

	if !validateListContents(listName1, []string{mockStream1.UserID, mockStream2.UserID}) ||
		!validateListContents(listName2, []string{mockStream2.UserID}) {
		t.Fatalf("The user's recent lists did not contain the correct stream user IDs")
	}
}

func TestGetUsersRecentStreamUserIDs(t *testing.T) {
	setup()
	defer teardown()

	mockUser := createRandomMockUser()
	listName := fmt.Sprintf("twitch_recent_streams:%s", mockUser.ID)
	mockStream1 := createRandomMockStream()
	mockStream2 := createRandomMockStream()
	mockStream3 := createRandomMockStream()

	if !validateListSize(listName, 0) {
		t.Fatalf("Error initial state, there are recent streams when it should be an empty list")
	}

	streamerUserID := getRecentStreamUserIDs(mockUser)
	if len(streamerUserID) != 0 {
		t.Fatalf("Should have returned nil slice when no active stream sessions, it did NOT")
	}

	SaveUsersCurrentStream(mockUser, mockStream1)
	if !validateListSize(listName, 1) {
		t.Fatalf("Failed to save first recent stream")
	}

	userIDs := getRecentStreamUserIDs(mockUser)

	if len(userIDs) == 0 {
		t.Fatal("Zero user IDs found in recent streams")
	}
	if userIDs[0] != mockStream1.UserID {
		t.Fatalf("List of streamer's user IDs is incorrect for recent streams with a single entry")
	}

	SaveUsersCurrentStream(mockUser, mockStream2)
	if !validateListSize(listName, 2) {
		t.Fatalf("Failed to save second recent stream, incorrect list size")
	}

	userIDs = getRecentStreamUserIDs(mockUser)
	if userIDs[0] != mockStream2.UserID ||
		userIDs[1] != mockStream1.UserID {
		t.Fatalf("List of streamer's user IDs is incorrect for recent streams with a second entry")
	}

	SaveUsersCurrentStream(mockUser, mockStream3)
	if !validateListSize(listName, 3) {
		t.Fatalf("Failed to save third recent stream, incorrect list size")
	}

	userIDs = getRecentStreamUserIDs(mockUser)
	if userIDs[0] != mockStream3.UserID ||
		userIDs[1] != mockStream2.UserID ||
		userIDs[2] != mockStream1.UserID {
		t.Fatalf("List of streamer's user IDs is incorrect for recent streams with a third entry")
	}
}

func TestGetUsersCurrentStream(t *testing.T) {
	setup()
	defer teardown()

	mockUser := createRandomMockUser()
	listName := fmt.Sprintf("twitch_recent_streams:%s", mockUser.ID)
	mockStream1 := createRandomMockStream()
	mockStream2 := createRandomMockStream()
	mockStream3 := createRandomMockStream()

	if !validateListSize(listName, 0) {
		t.Fatalf("Error initial state, there are recent streams when it should be an empty list")
	}

	streamerUserID := getCurrentStreamUserID(mockUser)
	if streamerUserID != "" {
		t.Fatalf("Should have returned empty string when no active stream sessions, it did NOT")
	}

	SaveUsersCurrentStream(mockUser, mockStream1)
	if !validateListSize(listName, 1) {
		t.Fatalf("Failed to save first recent stream")
	}

	streamerUserID = getCurrentStreamUserID(mockUser)
	if streamerUserID != mockStream1.UserID {
		t.Fatalf("Failed to retrieve new current stream after inserting one into list")
	}

	SaveUsersCurrentStream(mockUser, mockStream2)
	if !validateListSize(listName, 2) {
		t.Fatalf("Failed to save second recent stream, incorrect list size")
	}

	streamerUserID = getCurrentStreamUserID(mockUser)
	if streamerUserID != mockStream2.UserID {
		t.Fatalf("Failed to retrieve new current stream after inserting one into list")
	}

	SaveUsersCurrentStream(mockUser, mockStream3)
	if !validateListSize(listName, 3) {
		t.Fatalf("Failed to save third recent stream, incorrect list size")
	}

	streamerUserID = getCurrentStreamUserID(mockUser)
	if streamerUserID != mockStream3.UserID {
		t.Fatalf("Failed to retrieve new current stream after inserting one into list")
	}
}

func TestRemoveCurrentStream(t *testing.T) {

	setup()
	defer teardown()

	mockUser := createRandomMockUser()
	listName := fmt.Sprintf("twitch_recent_streams:%s", mockUser.ID)
	mockStream1 := createRandomMockStream()
	mockStream2 := createRandomMockStream()
	mockStream3 := createRandomMockStream()

	if !validateListSize(listName, 0) {
		t.Fatalf("Error bad initial test state, list of recent streams is NOT empty")
	}

	nextUID := removeCurrentStream(mockUser)
	if nextUID != "" {
		t.Fatalf("Error, removing a current stream should have returned empty string for the next UID but did not.")
	}

	SaveUsersCurrentStream(mockUser, mockStream1)
	SaveUsersCurrentStream(mockUser, mockStream2)
	SaveUsersCurrentStream(mockUser, mockStream3)

	nextUID = removeCurrentStream(mockUser)
	if nextUID != mockStream2.UserID {
		t.Fatalf("Error: incorrect next stream UID returned after removing current"+
			" stream from list. Expected=%s, Actual=%s", mockStream2.UserID, nextUID)
	}

	nextUID = removeCurrentStream(mockUser)
	if nextUID != mockStream1.UserID {
		t.Fatalf("Error: incorrect next stream UID returned after removing current"+
			" stream from list. Expected=%s, Actual=%s", mockStream1.UserID, nextUID)
	}

	nextUID = removeCurrentStream(mockUser)
	if nextUID != "" {
		t.Fatalf("Error: removed all current streams but still got a non-empty string for" +
			" the next stream UID value.")
	}
}

func createRandomMockUser() *User {
	seed := fmt.Sprintf("%d", rand.Intn(1000))
	return &User{
		ID:              "id" + seed,
		Login:           "login" + seed,
		DisplayName:     "DisplayName" + seed,
		Type:            "live",
		BroadcasterType: "partner",
	}
}

func createRandomMockStream() *TwitchStream {
	seed := fmt.Sprintf("%d", rand.Intn(1000))
	return &TwitchStream{
		ID:           "id" + seed,
		UserID:       "userID" + seed,
		CommunityIDs: nil,
		Type:         "live",
		Title:        "This is a stream that never ends...",
		ViewerCount:  rand.Int(),
		ThumbnailURL: "something.s3.somethingelse",
	}
}

func validateListSize(listName string, expected int) bool {

	return true
}

func validateListContents(listName string, expected []string) bool {

	return true
}

func clearRedisLists() {
	conn := redisConnPool.Get()
	defer conn.Close()
	reply, _ := redis.Strings(conn.Do("KEYS", "*"))

	for _, key := range reply {
		conn.Do("DEL", key)
	}
}

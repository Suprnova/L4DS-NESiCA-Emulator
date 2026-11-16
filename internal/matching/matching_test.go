package matching_test

import (
	"slices"
	"testing"

	"l4ds-nesica-emulator/internal/matching"
)

func TestParseQuery_ReadsTheTypeAndItems(t *testing.T) {
	datagram := []byte("2\x00" + `seqNo\7` + "\x00" + `version\1.2604` + "\x00" + `player_id\42` + "\xff")

	query, ok := matching.ParseQuery(datagram)

	if !ok || query.Type != '2' || query.Fields["seqNo"] != "7" || query.Fields["version"] != "1.2604" || query.Fields["player_id"] != "42" {
		t.Fatalf("ParseQuery() = %+v, %v", query, ok)
	}
}

func TestParseQuery_EmptyDatagram_IsRejected(t *testing.T) {
	_, ok := matching.ParseQuery(nil)

	if ok {
		t.Fatal("ParseQuery() ok = true")
	}
}

func TestReply_ListQuery_AnswersWithAnEmptyListForTheSequence(t *testing.T) {
	query := matching.Query{Type: '2', Fields: map[string]string{"seqNo": "258"}}

	got := matching.Reply(query)

	want := []byte{0, 0, 1, 2, 0x7e}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestReply_OtherQueries_NeedNoAnswer(t *testing.T) {
	got := matching.Reply(matching.Query{Type: '0', Fields: map[string]string{}})

	if got != nil {
		t.Fatalf("Reply() = %v, want nil", got)
	}
}

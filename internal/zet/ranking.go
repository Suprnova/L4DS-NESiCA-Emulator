package zet

import (
	"slices"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// The rankings the game asks for at startup, in the order of its ranking screen.
const (
	RankingNationalCoop   = 10
	RankingStoreCoop      = 20
	RankingNationalVersus = 30
	RankingStoreVersus    = 40
)

// rankingSize is how many players a ranking lists.
const rankingSize = 10

// Ranking answers a ranking request with the reply body: result, API_ID and the entry count,
// then per player rank, player id, score, title, name, level, class id, class name, store name,
// clan name and character, followed by the six avatar slots when the character is set.
//
// The co-op rankings are monthly, as the attract screen titles them: players are ordered by the
// total score of their results this calendar month (in the server's time zone), versus results
// left out, and show their co-op class. The versus rankings order the players who have played
// versus by versus class points over all time, show those as the score, and show their versus
// class. Each player shows their home store. The store rankings keep only the players whose home
// store is the asking cabinet's store; with no store (0), they equal the national ones. Titles,
// class names, clans and avatars are empty.
func (s *Server) Ranking(kind, store uint32) (string, error) {
	homeStore := int64(0)
	if kind == RankingStoreCoop || kind == RankingStoreVersus {
		homeStore = int64(store)
	}
	entries, err := s.rankingEntries(kind == RankingNationalVersus || kind == RankingStoreVersus, homeStore)
	if err != nil {
		return "", err
	}

	lines := []any{1, 1, len(entries)}
	for rank, entry := range entries {
		player := entry.player
		lines = append(lines, rank+1, player.ID, entry.score, "", player.Name, s.levels.Level(player.Exp), entry.class, "", s.storeName(player.HomeStore), "", player.CharaID)
		if player.CharaID != 0 {
			lines = append(lines, slices.Repeat([]any{0}, emptyAvatarLines)...)
		}
	}
	return lineBody(lines), nil
}

// rankingEntry is a ranked player with the score and class their entry shows.
type rankingEntry struct {
	player       store.Player
	score, class int64
}

// rankingEntries returns the entries of the versus ranking or the co-op ranking, best first,
// of the players of homeStore, or of every player when it is 0.
func (s *Server) rankingEntries(versus bool, homeStore int64) ([]rankingEntry, error) {
	if versus {
		players, err := s.store.TopVersusPlayers(homeStore, rankingSize)
		entries := make([]rankingEntry, len(players))
		for i, player := range players {
			entries[i] = rankingEntry{player, player.VSClassPoint, player.VSClassID}
		}
		return entries, err
	}

	now := s.now()
	scorers, err := s.store.TopScorersSince(time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()), homeStore, rankingSize)
	entries := make([]rankingEntry, len(scorers))
	for i, scorer := range scorers {
		entries[i] = rankingEntry{scorer.Player, scorer.Score, scorer.Player.CoopClassID}
	}
	return entries, err
}

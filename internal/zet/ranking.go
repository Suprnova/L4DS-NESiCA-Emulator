package zet

import "slices"

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
// clan name and character, followed by the six avatar slots when the character is set. The co-op
// rankings order players by score and show their co-op class; the versus rankings order the
// players who have played versus by versus class points, show those as the score, and show their
// versus class. Every player belongs to this server's one store, so the national and store
// rankings are the same. Titles, class names, clans and avatars are empty.
func (s *Server) Ranking(kind uint32) (string, error) {
	versus := kind == RankingNationalVersus || kind == RankingStoreVersus
	top := s.store.TopScorers
	if versus {
		top = s.store.TopVersusPlayers
	}
	players, err := top(rankingSize)
	if err != nil {
		return "", err
	}

	lines := []any{1, 1, len(players)}
	for rank, player := range players {
		score, class := player.Score, player.CoopClassID
		if versus {
			score, class = player.VSClassPoint, player.VSClassID
		}
		lines = append(lines, rank+1, player.ID, score, "", player.Name, s.levels.Level(player.Exp), class, "", s.storeName, "", player.CharaID)
		if player.CharaID != 0 {
			lines = append(lines, slices.Repeat([]any{0}, emptyAvatarLines)...)
		}
	}
	return lineBody(lines), nil
}

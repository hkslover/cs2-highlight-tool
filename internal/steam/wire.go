package steam

import "google.golang.org/protobuf/proto"

// The Game Coordinator speaks protobuf; these helpers are the only place that
// touches the protobuf runtime so the transport and the domain layer only deal
// with byte slices.

// marshalRecentGamesRequest builds the body of
// k_EMsgGCCStrike15_v2_MatchListRequestRecentUserGames.
func marshalRecentGamesRequest(accountID uint32) ([]byte, error) {
	request := &CMsgGCCStrike15V2_MatchListRequestRecentUserGames{
		Accountid: proto.Uint32(accountID),
	}
	return proto.Marshal(request)
}

// marshalFullGameInfoRequest builds the body of
// k_EMsgGCCStrike15_v2_MatchListRequestFullGameInfo. A share code carries
// exactly these three values.
func marshalFullGameInfoRequest(shareCode ShareCode) ([]byte, error) {
	request := &CMsgGCCStrike15V2_MatchListRequestFullGameInfo{
		Matchid:   proto.Uint64(shareCode.MatchID),
		Outcomeid: proto.Uint64(shareCode.ReservationID),
		Token:     proto.Uint32(uint32(shareCode.TVPort)),
	}
	return proto.Marshal(request)
}

func unmarshalMatchList(raw []byte, message *CMsgGCCStrike15V2_MatchList) error {
	return proto.Unmarshal(raw, message)
}

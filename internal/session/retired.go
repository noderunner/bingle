package session

import (
	"context"
	"fmt"
)

// RetiredRecord is what remains readable of a game whose kind was retired
// (catalog.Retire): who played, how it ended and how many moves were made.
// The moves themselves are still served by ListMoves, each with its
// notation; the position cannot be drawn without the game's definition.
type RetiredRecord struct {
	RoomCode   string
	GameDefID  string
	Name       string // the retired game's name
	Status     string
	MoveSeq    int
	WinnerSeat *int
	EndReason  string
	EndDetail  string
	Seats      []SeatInfo // the seats that were taken
}

// RetiredGame returns the record of a game of a retired kind. It is an error
// for a game whose kind is still offered (read that through GetState) or
// unknown.
func (m *Manager) RetiredGame(ctx context.Context, roomCode string) (RetiredRecord, error) {
	roomCode = canonRoom(roomCode)
	game, err := m.store.GetGame(ctx, roomCode)
	if err != nil {
		return RetiredRecord{}, err
	}
	name, ok := m.catalog.Retired(game.GameDefID)
	if !ok {
		return RetiredRecord{}, fmt.Errorf("session: room %q is not a game of a retired kind", roomCode)
	}
	players, err := m.store.ListPlayers(ctx, game.ID)
	if err != nil {
		return RetiredRecord{}, err
	}
	seats := make([]SeatInfo, 0, len(players))
	for _, p := range players {
		seats = append(seats, SeatInfo{Seat: p.Seat, Color: p.Color, Name: p.Name, Bot: p.Bot, PrincipalID: p.PrincipalID})
	}
	return RetiredRecord{
		RoomCode: game.RoomCode, GameDefID: game.GameDefID, Name: name, Status: game.Status, MoveSeq: game.MoveSeq,
		WinnerSeat: game.WinnerSeat, EndReason: game.EndReason, EndDetail: game.EndDetail, Seats: seats,
	}, nil
}

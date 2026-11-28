package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Announcement is one major announcement (ADR 0155): a title and a Markdown
// body that an administrator published, in the words they wrote it in.
type Announcement struct {
	ID          int64
	Title       string
	Body        string
	PublishedAt string
	Withdrawn   bool
	// Pictures are the names of its pictures, in order (#357).
	Pictures []string
}

// Picture is one picture of an announcement: a PNG or JPEG file its body
// shows by name.
type Picture struct {
	Name        string
	ContentType string
	Data        []byte
}

// CreateAnnouncement publishes an announcement at now, with its pictures,
// all at once: it is never offered without them.
func (s *Store) CreateAnnouncement(ctx context.Context, title, body string, pictures []Picture, now time.Time) (Announcement, error) {
	a := Announcement{Title: title, Body: body, PublishedAt: stamp(now)}
	tx, err := s.begin(ctx, "CreateAnnouncement")
	if err != nil {
		return Announcement{}, err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.exec(ctx, "CreateAnnouncement", `INSERT INTO announcements (title, body, published_at) VALUES (?, ?, ?)`, a.Title, a.Body, a.PublishedAt)
	if err != nil {
		return Announcement{}, err
	}
	if a.ID, err = res.LastInsertId(); err != nil {
		return Announcement{}, err
	}
	for _, p := range pictures {
		if _, err := tx.exec(ctx, "CreateAnnouncement", `INSERT INTO announcement_pictures (announcement_id, name, content_type, data) VALUES (?, ?, ?, ?)`, a.ID, p.Name, p.ContentType, p.Data); err != nil {
			return Announcement{}, err
		}
		a.Pictures = append(a.Pictures, p.Name)
	}
	return a, tx.Commit()
}

// DeleteAnnouncement deletes an announcement, its pictures and who has seen
// it. ErrNotFound when there is none by that id.
func (s *Store) DeleteAnnouncement(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, "DeleteAnnouncement", `DELETE FROM announcements WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// AnnouncementPicture is one picture of an announcement not withdrawn
// (ErrNotFound when there is none by that name, or it is withdrawn).
func (s *Store) AnnouncementPicture(ctx context.Context, id int64, name string) (Picture, error) {
	var p Picture
	err := s.queryRow(ctx, "AnnouncementPicture",
		`SELECT p.name, p.content_type, p.data FROM announcement_pictures p JOIN announcements a ON a.id = p.announcement_id
		 WHERE p.announcement_id = ? AND p.name = ? AND a.withdrawn = 0`, id, name).Scan(&p.Name, &p.ContentType, &p.Data)
	if errors.Is(err, sql.ErrNoRows) {
		return Picture{}, ErrNotFound
	}
	return p, err
}

// pictureNames are the names of the pictures of every announcement, by its id.
func (s *Store) pictureNames(ctx context.Context) (map[int64][]string, error) {
	rows, err := s.query(ctx, "AnnouncementPictureNames", `SELECT announcement_id, name FROM announcement_pictures ORDER BY announcement_id, rowid`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := map[int64][]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = append(out[id], name)
	}
	return out, rows.Err()
}

// WithdrawAnnouncement stops an announcement being offered or listed.
// ErrNotFound when there is none by that id.
func (s *Store) WithdrawAnnouncement(ctx context.Context, id int64) error {
	res, err := s.exec(ctx, "WithdrawAnnouncement", `UPDATE announcements SET withdrawn = 1 WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// Announcements lists the announcements, oldest first: those withdrawn too
// when withdrawn is true.
func (s *Store) Announcements(ctx context.Context, withdrawn bool) ([]Announcement, error) {
	q := `SELECT id, title, body, published_at, withdrawn FROM announcements`
	if !withdrawn {
		q += ` WHERE withdrawn = 0`
	}
	rows, err := s.query(ctx, "Announcements", q+` ORDER BY published_at, id`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Announcement
	for rows.Next() {
		var a Announcement
		if err := rows.Scan(&a.ID, &a.Title, &a.Body, &a.PublishedAt, &a.Withdrawn); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	_ = rows.Close()
	names, err := s.pictureNames(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Pictures = names[out[i].ID]
	}
	return out, nil
}

// AnnouncementByID is one announcement, withdrawn or not (ErrNotFound).
func (s *Store) AnnouncementByID(ctx context.Context, id int64) (Announcement, error) {
	var a Announcement
	err := s.queryRow(ctx, "AnnouncementByID", `SELECT id, title, body, published_at, withdrawn FROM announcements WHERE id = ?`, id).
		Scan(&a.ID, &a.Title, &a.Body, &a.PublishedAt, &a.Withdrawn)
	if errors.Is(err, sql.ErrNoRows) {
		return Announcement{}, ErrNotFound
	}
	if err != nil {
		return Announcement{}, err
	}
	names, err := s.pictureNames(ctx)
	a.Pictures = names[a.ID]
	return a, err
}

// AnnouncementsWaiting are the ids of the announcements waiting for a
// principal, oldest first: published after it was made, not withdrawn and not
// yet seen by it. Nobody is shown what was announced before they existed.
func (s *Store) AnnouncementsWaiting(ctx context.Context, principalID string) ([]int64, error) {
	rows, err := s.query(ctx, "AnnouncementsWaiting",
		`SELECT a.id FROM announcements a JOIN principals p ON p.id = ?
		 WHERE a.withdrawn = 0 AND a.published_at > p.created_at
		   AND NOT EXISTS (SELECT 1 FROM announcements_seen s WHERE s.principal_id = p.id AND s.announcement_id = a.id)
		 ORDER BY a.published_at, a.id`, principalID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// MarkAnnouncementsSeen records that the principal has closed the card that
// showed these announcements. An id with no announcement is passed over, and
// the first time each was seen is kept.
func (s *Store) MarkAnnouncementsSeen(ctx context.Context, principalID string, ids []int64, now time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.begin(ctx, "MarkAnnouncementsSeen")
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, id := range ids {
		if _, err := tx.exec(ctx, "MarkAnnouncementsSeen",
			`INSERT OR IGNORE INTO announcements_seen (principal_id, announcement_id, seen_at)
			 SELECT p.id, a.id, ? FROM principals p, announcements a WHERE p.id = ? AND a.id = ?`, stamp(now), principalID, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

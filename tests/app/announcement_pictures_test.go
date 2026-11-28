package app_test

// An announcement's pictures (#357): an administrator publishes them with it,
// anybody fetches them from its own path while it is not withdrawn, they are
// held to their limits, and they go when it is deleted.

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/noderunner/bingle/internal/app"
	"github.com/noderunner/bingle/internal/auth"
)

// picture is a file of an announcement's form.
type picture struct {
	name string
	data []byte
}

func pngOf(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	img.Set(1, 1, color.RGBA{200, 30, 30, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func jpegOf(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 6)), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// big is a PNG's signature and then n bytes: a PNG as far as the server's
// sniffing goes.
func big(n int) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), bytes.Repeat([]byte{7}, n)...)
}

// announceForm publishes an announcement as a multipart form, as the admin
// page does, a few seconds ahead of every guest made so far.
func announceForm(t *testing.T, a *app.App, b *browser, title, body string, pics []picture, out any) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("title", title)
	_ = mw.WriteField("body", body)
	for _, p := range pics {
		fw, _ := mw.CreateFormFile("pictures", p.name)
		_, _ = fw.Write(p.data)
	}
	_ = mw.Close()
	a.Resolver().SetClock(func() time.Time { return time.Now().Add(3 * time.Second) })
	defer a.Resolver().SetClock(time.Now)
	req, _ := http.NewRequest("POST", b.base+"/auth/admin/announcements", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set(auth.PageHeader, "1")
	resp, err := b.c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode
}

// fetchPicture gets a path and its answer's status, type and bytes.
func fetchPicture(t *testing.T, b *browser, path string) (int, string, []byte) {
	t.Helper()
	resp, err := b.c.Get(b.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), data
}

func TestAnnouncementPictures_AttachedAndServed(t *testing.T) {
	a, base, admin := announcementServer(t)
	shot, photo := pngOf(t), jpegOf(t)
	var made auth.Announcement
	if code := announceForm(t, a, admin, "Pictures", "![a board](board.png \"A board\")\n\n---\n\n![a photo](photo.jpg)", []picture{{"board.png", shot}, {"photo.jpg", photo}}, &made); code != http.StatusCreated {
		t.Fatalf("announce with pictures: %d", code)
	}
	if len(made.Pictures) != 2 || made.Pictures[0] != "board.png" || made.Pictures[1] != "photo.jpg" {
		t.Errorf("the answer's pictures: %v", made.Pictures)
	}
	guest := newBrowser(t, base)
	makeGuest(guest)
	if got := listed(guest); len(got) != 1 || len(got[0].Pictures) != 2 {
		t.Errorf("listed: %+v", got)
	}
	at := "/api/announcements/" + strconv.FormatInt(made.ID, 10) + "/pictures/"
	if code, kind, data := fetchPicture(t, guest, at+"board.png"); code != 200 || kind != "image/png" || !bytes.Equal(data, shot) {
		t.Errorf("board.png: %d %q, %d bytes", code, kind, len(data))
	}
	if code, kind, data := fetchPicture(t, guest, at+"photo.jpg"); code != 200 || kind != "image/jpeg" || !bytes.Equal(data, photo) {
		t.Errorf("photo.jpg: %d %q, %d bytes", code, kind, len(data))
	}
	for _, missing := range []string{at + "other.png", at + "..%2Fboard.png", "/api/announcements/999/pictures/board.png"} {
		if code, _, _ := fetchPicture(t, guest, missing); code != http.StatusNotFound {
			t.Errorf("%s: %d, want 404", missing, code)
		}
	}
	// Withdrawn, its pictures are not served.
	if code := admin.do("POST", "/auth/admin/announcements/"+strconv.FormatInt(made.ID, 10)+"/withdraw", nil, nil); code != 200 {
		t.Fatalf("withdraw: %d", code)
	}
	if code, _, _ := fetchPicture(t, guest, at+"board.png"); code != http.StatusNotFound {
		t.Errorf("a withdrawn one's picture: %d, want 404", code)
	}
}

func TestAnnouncementPictures_OnlyAdministratorsUpload(t *testing.T) {
	a, base, admin := announcementServer(t)
	pics := []picture{{"board.png", pngOf(t)}}
	guest := newBrowser(t, base)
	makeGuest(guest)
	if code := announceForm(t, a, guest, "News", "Text", pics, nil); code != http.StatusUnauthorized {
		t.Errorf("a guest uploads: %d, want 401", code)
	}
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	if code := announceForm(t, a, ana, "News", "Text", pics, nil); code != http.StatusForbidden {
		t.Errorf("an account uploads: %d, want 403", code)
	}
	if got := listed(admin); len(got) != 0 {
		t.Errorf("something was announced: %+v", got)
	}
}

func TestAnnouncementPictures_Limits(t *testing.T) {
	a, _, admin := announcementServer(t)
	for _, c := range []struct {
		what string
		pics []picture
		want int
	}{
		{"a picture over a megabyte", []picture{{"big.png", big(1 << 20)}}, http.StatusRequestEntityTooLarge},
		{"over four megabytes in all", []picture{{"a.png", big(900 << 10)}, {"b.png", big(900 << 10)}, {"c.png", big(900 << 10)}, {"d.png", big(900 << 10)}, {"e.png", big(900 << 10)}}, http.StatusRequestEntityTooLarge},
		{"thirteen pictures", func() []picture {
			var ps []picture
			for i := range 13 {
				ps = append(ps, picture{"p" + strconv.Itoa(i) + ".png", pngOf(t)})
			}
			return ps
		}(), http.StatusRequestEntityTooLarge},
		{"not a picture", []picture{{"page.png", []byte("<html><script>alert(1)</script>")}}, http.StatusBadRequest},
		{"a GIF", []picture{{"anim.png", []byte("GIF89a\x01\x00\x01\x00")}}, http.StatusBadRequest},
		{"a name with a space", []picture{{"my board.png", pngOf(t)}}, http.StatusBadRequest},
		{"a name that is not a picture's", []picture{{"board.svg", pngOf(t)}}, http.StatusBadRequest},
		{"one name twice", []picture{{"board.png", pngOf(t)}, {"board.png", pngOf(t)}}, http.StatusBadRequest},
	} {
		if code := announceForm(t, a, admin, "News", "Text", c.pics, nil); code != c.want {
			t.Errorf("%s: %d, want %d", c.what, code, c.want)
		}
	}
	if got := listed(admin); len(got) != 0 {
		t.Errorf("an announcement refused was published: %+v", got)
	}
	// Up to the limits is fine.
	pics := []picture{{"a.png", big(1<<20 - 100)}, {"b.png", big(1<<20 - 100)}, {"c.png", big(1<<20 - 100)}}
	if code := announceForm(t, a, admin, "News", "Text", pics, nil); code != http.StatusCreated {
		t.Errorf("three pictures of just under a megabyte: %d, want 201", code)
	}
}

func TestAnnouncementPictures_GoWithTheAnnouncement(t *testing.T) {
	a, base, admin := announcementServer(t)
	guest := newBrowser(t, base)
	makeGuest(guest)
	var made auth.Announcement
	if code := announceForm(t, a, admin, "Soon gone", "![x](board.png)", []picture{{"board.png", pngOf(t)}}, &made); code != http.StatusCreated {
		t.Fatalf("announce: %d", code)
	}
	id := strconv.FormatInt(made.ID, 10)
	if code, _, _ := fetchPicture(t, guest, "/api/announcements/"+id+"/pictures/board.png"); code != 200 {
		t.Fatalf("its picture: %d", code)
	}
	ana := newBrowser(t, base)
	ana.login("ana", anaPassword)
	if code := ana.do("POST", "/auth/admin/announcements/"+id+"/delete", nil, nil); code != http.StatusForbidden {
		t.Errorf("an account deletes one: %d, want 403", code)
	}
	if code := admin.do("POST", "/auth/admin/announcements/"+id+"/delete", nil, nil); code != http.StatusNoContent {
		t.Fatalf("delete: %d", code)
	}
	if code := admin.do("POST", "/auth/admin/announcements/"+id+"/delete", nil, nil); code != http.StatusNotFound {
		t.Errorf("deleting it again: %d, want 404", code)
	}
	if got := waitingFor(guest); len(got) != 0 {
		t.Errorf("a deleted one is waiting: %v", got)
	}
	var all auth.AnnouncementsResponse
	if code := admin.do("GET", "/auth/admin/announcements", nil, &all); code != 200 || len(all.Announcements) != 0 {
		t.Errorf("the admin's list: %d %+v", code, all)
	}
	if code, _, _ := fetchPicture(t, guest, "/api/announcements/"+id+"/pictures/board.png"); code != http.StatusNotFound {
		t.Errorf("a deleted one's picture: %d, want 404", code)
	}
}

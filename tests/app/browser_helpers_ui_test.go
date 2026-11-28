package app_test

// The browser helpers of browser_test.go that the other tests reach only in the private overlay.

import (
	"testing"
	"time"
)

// A press, a drag and a release arrive at the page as a mouse down, a move with the button
// held and a mouse up; a plain stepMouseMove is a move with no button.
func TestBrowserHelpers_APressDragAndReleaseReachThePage(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	if err := runSteps(ctx, stepNavigate(base+"/healthz"), stepEval(`window.seen = [];
		for (const type of ["mousedown", "mousemove", "mouseup"]) {
			document.addEventListener(type, (e) => seen.push([e.type, e.button, e.buttons, e.detail, e.clientX, e.clientY].join(":")));
		}
		true`)); err != nil {
		t.Fatal(err)
	}
	if err := runSteps(ctx,
		stepMouseMove(10, 12),
		stepMouseDown(20, 30),
		stepMouseDragTo(40, 50),
		stepMouseUp(40, 50),
	); err != nil {
		t.Fatal(err)
	}
	want := "mousemove:0:0:0:10:12 mousedown:0:1:1:20:30 mousemove:0:1:0:40:50 mouseup:0:0:1:40:50"
	if got := evalString(t, ctx, `seen.join(" ")`); got != want {
		t.Errorf("the page saw\n%s\nwant\n%s", got, want)
	}
}

// A phone on its side reports landscape to the page, one held upright reports portrait.
func TestBrowserHelpers_APhoneOnItsSideIsLandscape(t *testing.T) {
	base := newRESTTestServer(t)
	ctx, cancel := newChromedpContext(t, 30*time.Second)
	defer cancel()
	const orientation = `[screen.orientation.type, screen.orientation.angle, screen.width, screen.height].join(" ")`
	if err := runSteps(ctx, stepPhoneLandscape(932, 430, 1), stepNavigate(base+"/healthz")); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, orientation); got != "landscape-primary 90 932 430" {
		t.Errorf("on its side: %q", got)
	}
	if err := runSteps(ctx, stepPhone(430, 932, 1), stepNavigate(base+"/healthz")); err != nil {
		t.Fatal(err)
	}
	if got := evalString(t, ctx, orientation); got != "portrait-primary 0 430 932" {
		t.Errorf("upright: %q", got)
	}
}

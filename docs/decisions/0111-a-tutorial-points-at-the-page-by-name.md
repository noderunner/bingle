# 0111. A tutorial points at the page by name

Status: accepted (2026-11-05)

## Context

`highlight` and `arrow` point at the board. A big game's rules are also spread across the rest of the page:
Cthulhu Wars' Doom is a number in a cult's panel, its Ritual track is in the round bar, and its History is behind
a tab. A tutorial could only describe where these are, and the eight core tutorials of Cthulhu Wars need to show
them.

## Decision

A step's `spotlight` names the parts of the page to pick out. The names say what a part is (`power`, `phases`,
`history`), and `internal/tutorial` lists them, so an unknown name fails `go test ./tests/tutorials` as a square
off the board does. A part of a cult's panel is the learner's, or another seat's as `doom:2`.

The web page says where it has each name, in `tutorial.js`. On coming to a step it opens what the parts lie in and
brings them into view, once, so the learner can still look elsewhere. The step's card stays in view above them.

## Why not a CSS selector in the YAML

It would need no list, but it would tie every tutorial to the page's markup with nothing to check it outside a
browser, put markup in a document the API serves to every client, and mean nothing to a client other than the web
page. A name costs one line in each of two lists, and a test keeps them the same.

## Consequences

- The list has names that only Cthulhu Wars uses. It grows with the tutorials that need it, and a game without a
  part never names it.
- A part not on the page at that moment is passed over without complaint: `battle` before a battle, or a cult's
  panel on another of the phone's views. The browser test of the core tutorials checks that each step's parts are
  there on the desktop page.
- A tutorial cannot show the reaction settings, which a tutorial room leaves out because its script answers every
  reaction (`docs/tutorials.md`). It points at *Options* and says what is there.

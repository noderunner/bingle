# The map

![The map of this game|900](board.svg "The map of this game. Seas have blue gradients and waves, lands their own colours; the dark badge after a name is a glyph.")

The world comes in **six layouts**, four of Earth and two of the Primeval Map. You choose one when you create the game; the layout also decides how many cults sit
at the table (which cults, and whether the High Priests are in, is chosen separately).

| Layout | Seats | Areas | Variant id |
|---|---|---|---|
| 3 players | three cults | 13 | `cthulhu-wars-3p` |
| 4 players, **5-3** map | four cults — the Americas are split into five areas (North America West / East, Central America, South America West / East) and Australia has New Zealand beside it | 17 | `cthulhu-wars` |
| 4 players, **3-5** map | four cults — one North America and one South America, but Europe has Scandinavia, and Asia and Africa are cut into North / South Asia, Arabia, West / East Africa | 17 | `cthulhu-wars-3-5` |
| 5 players, 21-area map | five cults — both splits at once (the board made for five players) | 21 | `cthulhu-wars-5p` |
| 3 players, **Primeval** map | three cults, on the expansion's prehistoric Earth, with Glaciers (below) | 14 | `cthulhu-wars-primeval-3p` |
| 5 players, **Primeval** map | five cults, with Glaciers (below) | 22 | `cthulhu-wars-primeval-5p` |

A table of any layout can seat any of the six cults. A game made with other cults than the layout's own, or with the High
Priests, has an id of its own made of the layout and the choice: `cthulhu-wars-5p~ww-sl-gc-cc-ys-hp` means the
Windwalker, Sleeper, Great Cthulhu, Crawling Chaos and Yellow Sign in seats 1 to 5, with the High Priests.

> [!NOTE]
> The layouts are traced from the printed boards' areas and borders, with the coastlines drawn as straight lines.
> The areas, their names and which areas touch each other follow the boards; the exact outlines are simplified.

Units move along shared borders (Crawling Chaos flies two areas). The Pacific and the Indian Ocean continue
round both edges of the board: each is one area drawn in two pieces.

## Start areas

Each cult begins with a Gate and six Cultists in one area. The core cults begin where the tables below say; the
Windwalker and the Sleeper begin in the area marked with **their sigil** — a small badge after the area's name in the
colour of the cult.

## Sigils

| Sigil | Where | What it does |
|---|---|---|
| Windwalker | the Arctic Ocean and Antarctica, on every layout | The Windwalker begins in one of them, of its choice; Rhan-Tegoth awakens in either; Ithaqua replaces a Gate there; a Gate in the *other* one is one of its requirements. |
| Sleeper | North America (North America West where the board splits it) | The Sleeper begins there. |

A sigil is only shown while its cult is at the table.

## Glyphs

Glyphs sit beside an area's name, or on the 3-5 map where the board prints them. Yellow Sign needs to Desecrate one area of each kind:

| Glyph | Where |
|---|---|
| Dragon | the Americas (North America West / East, Central America, South America West / East where the board splits them) |
| Chevron | Africa and Australia (West / East Africa and New Zealand where the board splits them) |
| Thorns | Europe and Asia (Scandinavia, Arabia and North / South Asia where the board splits them) |

## Areas and borders

### 4 players, 5-3 map

17 areas, 42 borders. Variant id `cthulhu-wars`.

| Area | Type | Glyph | Start | Sigil | Borders |
|---|---|---|---|---|---|
| Africa | Land | Chevron | Black Goat |  | Asia, Indian Ocean, North Atlantic, South Atlantic |
| Antarctica | Land | — |  | Windwalker | Indian Ocean, South Atlantic, South Pacific |
| Arctic Ocean | Ocean | — |  | Windwalker | Asia, Europe, North America East, North America West, North Atlantic, North Pacific |
| Asia | Land | Thorns | Crawling Chaos |  | Africa, Arctic Ocean, Europe, Indian Ocean, North Atlantic, North Pacific |
| Australia | Land | Chevron |  |  | Indian Ocean, New Zealand |
| Central America | Land | Dragon |  |  | North America West, North Atlantic, North Pacific, South America West |
| Europe | Land | Thorns | Yellow Sign |  | Arctic Ocean, Asia, North Atlantic |
| Indian Ocean | Ocean | — |  |  | Africa, Antarctica, Asia, Australia, New Zealand, North Pacific, South Atlantic, South Pacific |
| New Zealand | Land | Chevron |  |  | Australia, Indian Ocean, South Pacific |
| North America East | Land | Dragon |  |  | Arctic Ocean, North America West, North Atlantic |
| North America West | Land | Dragon |  | Sleeper | Arctic Ocean, Central America, North America East, North Atlantic, North Pacific |
| North Atlantic | Ocean | — |  |  | Africa, Arctic Ocean, Asia, Central America, Europe, North America East, North America West, South America East, South Atlantic |
| North Pacific | Ocean | — |  |  | Arctic Ocean, Asia, Central America, Indian Ocean, North America West, South America West, South Pacific |
| South America East | Land | Dragon |  |  | North Atlantic, South America West, South Atlantic |
| South America West | Land | Dragon |  |  | Central America, North Pacific, South America East, South Atlantic, South Pacific |
| South Atlantic | Ocean | — |  |  | Africa, Antarctica, Indian Ocean, North Atlantic, South America East, South America West, South Pacific |
| South Pacific | Ocean | — | Great Cthulhu |  | Antarctica, Indian Ocean, New Zealand, North Pacific, South America West, South Atlantic |

### 4 players, 3-5 map

17 areas, 44 borders. Variant id `cthulhu-wars-3-5`.

| Area | Type | Glyph | Start | Sigil | Borders |
|---|---|---|---|---|---|
| Antarctica | Land | — |  | Windwalker | Indian Ocean, South Atlantic, South Pacific |
| Arabia | Land | Thorns |  |  | East Africa, Europe, Indian Ocean, North Asia, North Atlantic, South Asia, West Africa |
| Arctic Ocean | Ocean | — |  | Windwalker | North America, North Asia, North Atlantic, North Pacific, Scandinavia |
| Australia | Land | Chevron |  |  | Indian Ocean, South Pacific |
| East Africa | Land | Chevron |  |  | Arabia, Indian Ocean, South Atlantic, West Africa |
| Europe | Land | Thorns | Yellow Sign |  | Arabia, North Asia, North Atlantic, Scandinavia |
| Indian Ocean | Ocean | — |  |  | Antarctica, Arabia, Australia, East Africa, North Pacific, South Asia, South Atlantic, South Pacific |
| North America | Land | Dragon |  | Sleeper | Arctic Ocean, North Atlantic, North Pacific, South America |
| North Asia | Land | Thorns |  |  | Arabia, Arctic Ocean, Europe, North Pacific, Scandinavia, South Asia |
| North Atlantic | Ocean | — |  |  | Arabia, Arctic Ocean, Europe, North America, Scandinavia, South America, South Atlantic, West Africa |
| North Pacific | Ocean | — |  |  | Arctic Ocean, Indian Ocean, North America, North Asia, South America, South Asia, South Pacific |
| Scandinavia | Land | Thorns |  |  | Arctic Ocean, Europe, North Asia, North Atlantic |
| South America | Land | Dragon |  |  | North America, North Atlantic, North Pacific, South Atlantic, South Pacific |
| South Asia | Land | Thorns | Crawling Chaos |  | Arabia, Indian Ocean, North Asia, North Pacific |
| South Atlantic | Ocean | — |  |  | Antarctica, East Africa, Indian Ocean, North Atlantic, South America, South Pacific, West Africa |
| South Pacific | Ocean | — | Great Cthulhu |  | Antarctica, Australia, Indian Ocean, North Pacific, South America, South Atlantic |
| West Africa | Land | Chevron | Black Goat |  | Arabia, East Africa, North Atlantic, South Atlantic |

### 3 players

13 areas, 32 borders. Variant id `cthulhu-wars-3p`.

| Area | Type | Glyph | Start | Sigil | Borders |
|---|---|---|---|---|---|
| Africa | Land | Chevron | Black Goat |  | Asia, Indian Ocean, North Atlantic, South Atlantic |
| Antarctica | Land | — |  | Windwalker | Indian Ocean, South Atlantic, South Pacific |
| Arctic Ocean | Ocean | — |  | Windwalker | Asia, Europe, North America, North Atlantic, North Pacific |
| Asia | Land | Thorns | Crawling Chaos |  | Africa, Arctic Ocean, Europe, Indian Ocean, North Atlantic, North Pacific |
| Australia | Land | Chevron |  |  | Indian Ocean, South Pacific |
| Europe | Land | Thorns |  |  | Arctic Ocean, Asia, North Atlantic |
| Indian Ocean | Ocean | — |  |  | Africa, Antarctica, Asia, Australia, North Pacific, South Atlantic, South Pacific |
| North America | Land | Dragon |  | Sleeper | Arctic Ocean, North Atlantic, North Pacific, South America |
| North Atlantic | Ocean | — |  |  | Africa, Arctic Ocean, Asia, Europe, North America, South America, South Atlantic |
| North Pacific | Ocean | — |  |  | Arctic Ocean, Asia, Indian Ocean, North America, South America, South Pacific |
| South America | Land | Dragon |  |  | North America, North Atlantic, North Pacific, South Atlantic, South Pacific |
| South Atlantic | Ocean | — |  |  | Africa, Antarctica, Indian Ocean, North Atlantic, South America, South Pacific |
| South Pacific | Ocean | — | Great Cthulhu |  | Antarctica, Australia, Indian Ocean, North Pacific, South America, South Atlantic |

### 5 players, 21-area map

21 areas, 54 borders. Variant id `cthulhu-wars-5p`.

| Area | Type | Glyph | Start | Sigil | Borders |
|---|---|---|---|---|---|
| Antarctica | Land | — |  | Windwalker | Indian Ocean, South Atlantic, South Pacific |
| Arabia | Land | Thorns |  |  | East Africa, Europe, Indian Ocean, North Asia, North Atlantic, South Asia, West Africa |
| Arctic Ocean | Ocean | — |  | Windwalker | North America East, North America West, North Asia, North Atlantic, North Pacific, Scandinavia |
| Australia | Land | Chevron |  |  | Indian Ocean, New Zealand |
| Central America | Land | Dragon |  |  | North America West, North Atlantic, North Pacific, South America West |
| East Africa | Land | Chevron |  |  | Arabia, Indian Ocean, South Atlantic, West Africa |
| Europe | Land | Thorns | Yellow Sign |  | Arabia, North Asia, North Atlantic, Scandinavia |
| Indian Ocean | Ocean | — |  |  | Antarctica, Arabia, Australia, East Africa, New Zealand, North Pacific, South Asia, South Atlantic, South Pacific |
| New Zealand | Land | Chevron |  |  | Australia, Indian Ocean, South Pacific |
| North America East | Land | Dragon |  |  | Arctic Ocean, North America West, North Atlantic |
| North America West | Land | Dragon |  | Sleeper | Arctic Ocean, Central America, North America East, North Atlantic, North Pacific |
| North Asia | Land | Thorns |  |  | Arabia, Arctic Ocean, Europe, North Pacific, Scandinavia, South Asia |
| North Atlantic | Ocean | — |  |  | Arabia, Arctic Ocean, Central America, Europe, North America East, North America West, Scandinavia, South America East, South America West, South Atlantic, West Africa |
| North Pacific | Ocean | — |  |  | Arctic Ocean, Central America, Indian Ocean, North America West, North Asia, South America West, South Asia, South Pacific |
| Scandinavia | Land | Thorns |  |  | Arctic Ocean, Europe, North Asia, North Atlantic |
| South America East | Land | Dragon |  |  | North Atlantic, South America West, South Atlantic |
| South America West | Land | Dragon |  |  | Central America, North Atlantic, North Pacific, South America East, South Pacific |
| South Asia | Land | Thorns | Crawling Chaos |  | Arabia, Indian Ocean, North Asia, North Pacific |
| South Atlantic | Ocean | — |  |  | Antarctica, East Africa, Indian Ocean, North Atlantic, South America East, South Pacific, West Africa |
| South Pacific | Ocean | — | Great Cthulhu |  | Antarctica, Indian Ocean, New Zealand, North Pacific, South America West, South Atlantic |
| West Africa | Land | Chevron | Black Goat |  | Arabia, East Africa, North Atlantic, South Atlantic |

## The Primeval Map and its Glaciers

The Primeval Map's main mechanic is the slow arrival of Glaciers, which abandon Gates for good and so make the map rich in Power. The three-player layout's outlines were drawn by eye from pictures of the printed board; the five-player layout is traced from a scan, and shows the board's Glacier glyphs, which you can hide in the game options.

- At the end of the **first Doom phase**, after everyone has had the chance to perform a Ritual, a Glacier appears in each cult's start area, whether or not it has a Glacier glyph. Any unit holding a Gate there stays in the area, and the Gate is abandoned.
- At the end of **each later Doom phase**, the first player chooses **two areas with a Glacier glyph** that have no Glacier yet. Once every such area has one, no more are placed. Glaciers never move or leave.
- A Gate under a Glacier is **abandoned for good**. Nobody can ever control it, and it gives every cult Power in Gather Power, like any abandoned Gate. Creating a Gate in a Glacier's area gets the same result straight away.
- Yog-Sothoth, who is a Gate himself, is not covered.

On the map a Glacier is a ❄ chip in its area. On this map, every area with "Ocean" in its name is an ocean, and no others.

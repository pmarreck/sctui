---
description: "Bubble Tea reserves space as KeySpace so focused text input needs explicit routing."
datetime: 2026-07-17T14:56:26-04:00 # America/New_York (EDT)
tags: [bubble, tea, reserves, space, keyspace, focused, text, input, explicit, routing]
---
Bubble Tea delivers a pressed space as `tea.KeySpace`, not `tea.KeyRunes`.
Because sctui handles global playback shortcuts before routing keys to the
active view, focused text input must explicitly own `KeySpace` and append it.
Keep the player fallback outside `search.StateInput`, including while Search
results are visible, so fixing text entry does not disable play/pause.

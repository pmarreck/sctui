# Protected Playback Architecture Spike

Date: 2026-07-23 EDT

## Scope

This was an observational test of normal, logged-in Firefox playback. It did
not collect credentials, signed media URLs, licenses, keys, encrypted media, or
decoded audio.

## Candidates

- `Bio-Mechanic` by Front Line Assembly
  (`frontlineassemblyofficial/bio-mechanic-1`, track `718243153`)
- `I Wanna Be Adored (Remastered 2009)` by The Stone Roses
  (`the-stone-roses/i-wanna-be-adored-remastered`, track `253548568`)

Both records are `SUB_HIGH_TIER`. The signed-in direct-client diagnostic found
encrypted HLS variants for each candidate and received HTTP 404 for all of
their ordinary HLS/progressive variants.

The encrypted manifests advertise Apple streaming-key delivery for CBCS and
Widevine/PlayReady key-system identifiers for CENC. The diagnostic deliberately
redacts signed URLs, key URLs, key identifiers, IVs, and opaque protection data.

## Browser Observation

Firefox Beta was launched normally with Peter's existing profile and its local
WebDriver BiDi endpoint. A temporary SoundCloud tab registered an observer that
recorded only EME key-system selections and encrypted-media event metadata.

When the `Bio-Mechanic` page's global playback control was activated:

- SoundCloud requested `com.widevine.alpha`, `com.microsoft.playready`, and
  `com.apple.fps`.
- Firefox rejected PlayReady and FairPlay as unsupported.
- Firefox granted Widevine for several capability requests, while rejecting
  other Widevine configurations with `NotSupportedError`.

Firefox then advanced to a different, non-DRM queue item instead of continuing
with the Front Line Assembly candidate. This matches `sctui`'s skip behavior
when it cannot play a protected candidate.

No key, license, media URL, encrypted segment, decrypted buffer, or raw
protection blob was collected.

## Conclusion

The direct-client evidence remains conclusive for the exact track records:
their available, signed-in transcodings are encrypted HLS and their ordinary
transcodings are denied. The browser tried the corresponding EME key systems,
then skipped the candidate in this Firefox environment. This is runtime evidence
of a DRM path, not successful browser playback of either candidate.

`sctui`'s direct ffmpeg decoder cannot support this path. Running the SoundCloud
page code under Node would not supply the CDM capability it requests. A
browser-backed control adapter could support only tracks that the chosen browser
and its CDM can actually play. No protected-media extraction or decryption work
is in scope.

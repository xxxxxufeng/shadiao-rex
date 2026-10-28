# Test fixtures (not committed)

`cenc` and `api` tests skip when these are missing.

1. Get one encrypted 720p episode's key and URL with openhongguo (`sdk.ResolvePlayback` + `sdk.ContentKey`).
2. `curl -o testdata/ep.mp4 "<url>"` and write the hex key to `testdata/ep.key`.
3. `ffmpeg -decryption_key $(cat testdata/ep.key) -i testdata/ep.mp4 -c copy testdata/ep.dec.mp4`

Tests assume the 2026-10-28 fixture (《沙雕女主荒岛爆笑求生记》ep 1, 720p):
moov at [28, 134265), 3206 video + 2305 audio samples.

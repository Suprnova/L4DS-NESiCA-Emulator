# L4DS NESiCA Emulator

A server for the online services of the defunct arcade game *Left 4 Dead Survivors* (Taito/Neilo, 2014).

> **Do not expose this to the open internet.** The game runs a very old, vulnerable build of Source, and the protocols it speaks are unauthenticated and unencrypted. Supported use is closed networks or very small groups of trusted people, for example over a virtual LAN.

## Components

- `l4ds-server`: HTTP game-server backend, UDP lobby and database.
- `l4ds-shim`: stand-in for Taito's `NesysService.exe` on each player's PC; forwards to a server.
- `l4ds-aio`: server and shim in one process, for local development.

## Status

Early development. Only the pipe protocol framing exists so far.

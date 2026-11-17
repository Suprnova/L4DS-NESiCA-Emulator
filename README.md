# L4DS NESiCA Emulator

A server for the online services of the defunct arcade game *Left 4 Dead Survivors* (Taito/Neilo, 2014).

> **Do not expose this to the open internet.** The game runs a very old, vulnerable build of Source, and the protocols it speaks are unauthenticated and unencrypted. Supported use is closed networks or very small groups of trusted people, for example over a virtual LAN.

## Components

- `l4ds-server`: HTTP game-server backend, UDP lobby and database.
- `l4ds-shim`: stand-in for Taito's `NesysService.exe` on each player's PC; forwards to a server.
- `l4ds-aio`: server and shim in one process, for local development.
- `l4ds-tree`: builds patched private copies of the game ("trees"): a server tree that `l4ds-aio` runs as dedicated servers, and client trees that run the game with cards inserted from the console. The installed game is never written to.
- `srcds-stub`: the `srcds.exe` of a server tree; `l4ds-tree` builds it.

## Status

In development. `l4ds-aio` runs on Windows next to the game. With it, the unmodified game reaches its online mode: card registration and login, single-player chapters with saved progress and settings, and national co-op, whose rooms are hosted by dedicated servers that `l4ds-aio` runs from a server tree. Store co-op, versus and events are not working yet.

## Running

All commands run from the repository on Windows, with Go on the `PATH`.

Build the trees once (the folders must not exist yet):

```
go run ./cmd/l4ds-tree build -game-dir <game folder> -out <server tree>
go run ./cmd/l4ds-tree build -game-dir <game folder> -out <client tree> -client
```

Run the server:

```
go run ./cmd/l4ds-aio -game-dir <game folder> -ds-dir <server tree>
```

Run the game from the client tree with the usual arcade command line, and put a card on the reader from the game's console:

```
<client tree>\left4dead2.exe -console -arcadeIO_InitializeSkip -game left4dead2 -language japanese -windowed
change_debug_dummyCardID <16-digit card number>
change_debug_dummyCardID 0
```

The first console command puts that card on the reader, the second takes it off. The installed game also works, with `-simulateNESICA <card>` in place of the console commands, but then the card stays on the reader for the whole run and the startup and attract movies never play.

`go run ./cmd/l4ds-tree revert <tree>` restores a tree's original bytes, and `apply` patches it again. Never delete a tree with a recursive delete that follows junctions, such as Windows PowerShell 5.1's `Remove-Item -Recurse`: most of a tree is junctions to the game's folders, and their contents would go too.

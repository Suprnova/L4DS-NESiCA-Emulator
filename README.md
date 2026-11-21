# L4DS NESiCA Emulator

A server for the online services of the defunct arcade game *Left 4 Dead: Survivors* (*LEFT 4 DEAD -生存者たち-*, also known as L4DAC; Taito/Neilo, 2014), build 1.26. The unmodified game connects to it as it once connected to Taito's NESiCA network and Square Enix's game servers: cards register and log in, progress is saved, and cabinets find each other for co-op, versus, and events.

> **Do not expose this to the open internet.** The game runs a very old, vulnerable build of Source, and the protocols it speaks are unauthenticated and unencrypted. Supported use is closed networks or very small groups of trusted people, for example over a virtual LAN.

## Status

In development. Everything runs on one Windows PC today, through `l4ds-aio`:

- card registration, login, saved progress, classes, and settings;
- carried weapons and skill items, and gifts handed out at login;
- single play, national co-op, store co-op, Versus Survival, and events, with up to two copies of the game on the same PC;
- rankings from the saved players on the attract screen.

Playing across several PCs needs a separate server and a small service on each player's PC; both are planned, and so are prebuilt releases. Until the first release, you build from source.

## What you need

- Windows.
- Your own copy of *Left 4 Dead: Survivors* 1.26 (version `1.2604`). The tools never write to it.
- [Go](https://go.dev/dl/), on the `PATH`, to build from source.

## Setting up one PC

Run these from the repository.

1. **Build the trees.** A tree is a private copy of the game: copies of the files the tools patch or the game writes, and junctions to the game's other folders, so it takes little space. The server tree runs the game as the dedicated servers that host rooms; a client tree runs the game with cards inserted from its console. Each folder must not exist yet, and the server tree's path must not contain spaces.

   ```
   go run ./cmd/l4ds-tree build -game-dir <game folder> -out <server tree>
   go run ./cmd/l4ds-tree build -game-dir <game folder> -out <client tree> -client
   ```

   For a second copy of the game on the same PC, build a second client tree.

2. **Point the game at the server.** The game finds its matching server by a host name, so that name must resolve to the PC running the server. Add this line to `C:\Windows\System32\drivers\etc\hosts` (an administrator must edit it):

   ```
   127.0.0.1 dev.game.zet.square-enix.co.jp
   ```

   The name is the `server_key` in the configuration file; if you change one, change the other. A DNS server on your network that resolves the name works too, in place of the hosts file.

3. **Configure the server.** Copy `l4ds.example.yaml` and adjust it: in the `server` section the store's name and prefecture and the calendar file; in the `shim` section the server key, the netcon port, and your card keys. Copy `calendar.example.yaml` too: it holds what the server offers and when, such as the versus schedule, the events, the gifts handed out at login, campaigns (daily bonus SVP, login ladders, returning-player packs, and rewards for plays), the national co-op stage list, and maintenance, and entries can repeat (every week, every year). Every setting is described in the two files. Without a calendar nothing is scheduled, and versus and the event modes stay unavailable.

4. **Start the server** and wait for `dedicated server ready` before creating a room:

   ```
   go run ./cmd/l4ds-aio -ds-dir <server tree> -config <configuration file>
   ```

   Players are saved in `l4ds.db` in the current folder (`-db` changes it). `go run ./cmd/l4ds-aio -help` lists every option.

5. **Start the game** from the client tree:

   ```
   <client tree>\left4dead2.exe -console -arcadeIO_InitializeSkip -game left4dead2 -language japanese -windowed -netconport 2121
   ```

   `-netconport` opens the port card keys send cards to; it must match `netcon` in the `shim` section.

6. **Scan a card.** Any 16-digit number is a card; a new one starts registration. Press a card key from the `shim` section's `cards` while the game shows its title or card screen: the card is read once, then taken off the reader again. Without card keys, put a card on the reader from the game's console:

   ```
   change_debug_dummyCardID <16-digit card number>
   ```

## Gifts for one card

`go run ./cmd/l4ds-server grant` queues a gift for the player of one card, delivered in the reward pop-up at the card's next login, while the server runs or not. For example, 1000 ZG and two skill items (the id is the item's in the game's catalogue):

```
go run ./cmd/l4ds-server grant -card <16-digit card number> -name おわび -zet 1000 -item skill_item -item-id 21 -item-name 胆汁ガード -count 2
```

`-item` is `weapon`, `skill_item`, `svp`, `title`, or an avatar item (`upper`, `lower`, `head`, or `accessory`, named with `-unit` and `-color` as the grants in `calendar.example.yaml` describe); `go run ./cmd/l4ds-server grant -h` lists every flag.

## Trees

`go run ./cmd/l4ds-tree revert <tree>` restores a tree's original bytes, and `apply` patches it again. Most of a tree is junctions to the game's folders, so delete a tree with `cmd /c rmdir /s /q <tree>`, which removes junctions without following them into the game.

## License

The L4DS NESiCA Emulator is licensed under the [GNU Affero General Public License v3.0 or later](LICENSE). Running it unmodified asks nothing of you. If you modify it and players use your modified version, through cabinets or over a network, you must offer them its source.

## Contributing

Contributions are welcome under the terms of the [AI contributions policy](AI_POLICY.md).

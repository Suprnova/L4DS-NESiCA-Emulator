// Package shim serves the game's NESYS pipe in place of Taito's NesysService, with a backend
// that answers the game's network requests: a server in the same process for l4ds-aio, or a
// remote server for l4ds-shim, whose [Link] tells the game when that server goes away and comes
// back.
package shim

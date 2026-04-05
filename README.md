# go-ObuShun

*Inspired by and frontend UI ported from the original [yukimemi/shun](https://github.com/yukimemi/shun) project.*
*See [CREDITS.md](./CREDITS.md) for full license details of the ported frontend code and third-party libraries.*

## About

This project is a Wails (Go + Svelte) implementation of a minimal, keyboard-driven launcher based on `shun`.

## Live Development

To run in live development mode, run `wails dev` in the project directory. This will run a Vite development
server that will provide very fast hot reload of your frontend changes. If you want to develop in a browser
and have access to your Go methods, there is also a dev server that runs on http://localhost:34115. Connect
to this in your browser, and you can call your Go code from devtools.

## Building

To build a redistributable, production mode package, use `wails build`.

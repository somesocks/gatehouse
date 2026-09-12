import "katex/dist/katex.min.css"
import "@fontsource/ibm-plex-sans/500.css"
import "@fontsource/ibm-plex-sans-condensed/500.css"
import "@fontsource/ibm-plex-sans-condensed/600.css"
import "./app.css"
import { mount } from "svelte"

import App from "./App.svelte"

const target = document.querySelector<HTMLElement>("#app")

if (target === null) {
  throw new Error("missing application root")
}

mount(App, { target })

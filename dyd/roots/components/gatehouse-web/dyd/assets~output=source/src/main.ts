import "bulma/css/bulma.min.css"
import "katex/dist/katex.min.css"
import "./app.css"
import { mount } from "svelte"

import App from "./App.svelte"

const target = document.querySelector<HTMLElement>("#app")

if (target === null) {
  throw new Error("missing application root")
}

mount(App, { target })

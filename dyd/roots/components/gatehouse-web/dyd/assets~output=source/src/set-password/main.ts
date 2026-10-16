import "../app.css"
import { mount } from "svelte"
import SetPassword from "./SetPassword.svelte"

const target = document.querySelector<HTMLElement>("#set-password")
if (target === null) throw new Error("missing set-password root")
mount(SetPassword, { target })

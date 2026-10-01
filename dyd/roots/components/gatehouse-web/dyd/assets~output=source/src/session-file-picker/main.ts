import "../gatehouse.css"
import "../app.css"
import { mount } from "svelte"
import SessionFilePicker from "./SessionFilePicker.svelte"

const target = document.querySelector<HTMLElement>("#session-file-picker")
if (target === null) throw new Error("missing session file picker root")
mount(SessionFilePicker, { target })

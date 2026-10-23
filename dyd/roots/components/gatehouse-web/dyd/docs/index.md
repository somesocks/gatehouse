# Gatehouse web documentation

Start here when designing, implementing, or maintaining the Gatehouse web UI.

## Sections

| Section | Contents |
| --- | --- |
| [1. Design](01-design.md) | Design requirements for layout, consistency, resource identity, navigation, visual conventions, actions, feedback, forms, accessibility, and language. |
| [2. Patterns](02-patterns.md) | Page and component composition, chat, dialogs, selectors, management sections, and the page-workflow inventory. |
| [3. Architecture](03-architecture.md) | Presentation boundary, component ownership, styling, navigation, transport, and theming. |
| [4. Lifecycle](04-lifecycle.md) | Request ownership, cancellation, snapshots, drafts, conflicts, refresh, and navigation-state retention. |
| [5. Development](05-development.md) | Dryad environments, dependency maintenance, builds, type checking, and tests. |

## Reading order

- For UI changes, read **Design**, then the relevant **Patterns** sections.
- For implementation changes, read **Architecture** and **Lifecycle** as well.
- Use **Development** for the package's build and verification commands.

Each rule has a canonical home. Pattern-specific rules state their scope;
full-page layouts, chat-resource dialogs, and capability tools have distinct
contracts. When documentation and implementation disagree, identify whether the
text is stale or the implementation has drifted. New design recommendations
should be identified as proposals until adopted.

## External reference

[Bits UI reference](bits-ui.md) contains third-party component documentation.

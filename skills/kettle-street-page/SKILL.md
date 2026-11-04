---
name: kettle-street-page
description: Build, update, or review the Kettle Street Cycle Works landing page (bike-shop/index.html). Use this whenever the user mentions Kettle Street, the bike shop site or landing page, the repair price list, booking or walk-in sections, or wants to change copy, prices, hours, or styling on that page, even if they don't name the file or say "landing page."
---

# Kettle Street Cycle Works page

This skill keeps changes to the Kettle Street landing page consistent with the design plan it was built from. The page lives in `bike-shop/index.html` as one self-contained file with inline CSS.

## Design plan to keep

The page is distinctive because of a few specific choices. Keep them unless the user asks to change them, since drifting toward generic defaults is what makes a page look templated.

- **Palette:** tire black `#1B1C1A`, sidewall white `#F3F3EF`, frame enamel teal `#1F5B57`, steel grey `#6F767C`, and brass `#B8862F`. Brass is reserved for the main booking button, so it stays rare enough to mean "act here".
- **Type:** Big Shoulders Display for headings and the sidewall band, Newsreader for body text. Keep body line length under about 80 characters and keep the extra line height on body text.
- **Layout:** Left-aligned throughout. Services stay in a table, not a card grid. Numbers appear only in the three-step visit process, because that is a real sequence.
- **Sidewall band:** The full-width dark band of tire-lettering words is the one memorable graphic element. Don't add a second decorative element that competes with it.

## Avoid these defaults

- A cream or warm off-white background, or terracotta accents.
- Eyebrow labels above headings, and ALL-CAPS labels on small text.
- Middle-dot strings like "A · B · C" and arrows (→) on links or buttons.
- Identical rounded cards, gradient washes, or fade-in on every section.

## Content rules

- Write copy from the customer's point of view in plain sentences. Name things the way a rider would say them, not the way the shop's system stores them.
- Prices, hours, address, and phone number are placeholders until the user supplies real ones. Say so when you edit them, and don't present placeholders as real.
- Quote the prices in pounds, and keep British spelling (for example "tyre", "labour").

## Making a change

1. Read `bike-shop/index.html` before editing so the change matches the existing structure and class names.
2. Make targeted edits. Don't rewrite the whole file unless asked.
3. Check that the page still works on a narrow screen. The nav hides all but the last link below 40rem, and the steps and visit grid stack.
4. Keep focus styles and `prefers-reduced-motion` handling in place. Don't add motion beyond what the page already has.
5. Tell the user what changed, and flag any placeholder that now needs real details.

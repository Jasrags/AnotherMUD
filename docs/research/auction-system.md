# Aardwolf — Auction System

Source: https://www.aardwolf.com/mud/auction.html
Fetched: 2026-06-09

## Overview

There are two automated auction systems in place on Aardwolf:

1. **Normal Equipment Auction** — runs 24/7. Players place items on the auction block any
   time for others to bid on. Players can check the list of currently auctioned items and bid
   on something they desire. Currency: gold.

2. **Remort Auction** — tied in with the remort system. Allows special quest equipment
   (purchasable via questing) to be auctioned. Currency: quest points, not gold. These
   auctions only take place when a superhero chooses to remort and decides to auction off
   special equipment they do not wish to carry through the remort process. Only one special
   item is ever auctioned at a time. All bids are visible on the remort auction channel.

## Key Design Notes

- Dual-currency auction (gold for normal gear, quest points for remort gear)
- Remort auction is event-triggered (player choosing to remort), not always-on
- Remort auction has its own dedicated channel
- Normal auction is persistent 24/7

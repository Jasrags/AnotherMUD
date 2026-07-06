# starter-world — Gazetteer

Region → area → room reference for the `starter-world` content pack — 13 rooms across 2 areas. Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.

## Unassigned region

### Hearthwick Village (`town` · weather: temperate)

#### Hearthwick Forge `forge`

- Terrain: indoors
- Notes: craft station
- Exits:
    - south → town-square
    - west → forge-nook (hidden)
    - down → forge-cellar (door: a sturdy oak door)
- NPCs:
    - Maerys the Training Master (trainer, quest giver)
    - Brandr the blacksmith (shop, trainer)

#### Forge Cellar `forge-cellar`

- Terrain: underground
- Notes: items present
- Exits:
    - up → forge (door: a sturdy oak door)
    - down → forge-vault (locked door: an iron door)

#### Hidden Alcove `forge-nook`

- Terrain: indoors
- Exits:
    - east → forge

#### Forge Vault `forge-vault`

- Terrain: underground
- Notes: items present
- Exits:
    - up → forge-cellar (locked door: an iron door)

#### Market Row `market`

- Terrain: —
- Notes: craft station
- Exits:
    - west → town-square
- NPCs:
    - Marta the cook (shop, trainer)

#### Town Square `town-square`

- Terrain: —
- Notes: start room, items present
- Exits:
    - north → forge
    - south → village-gate (cross-area)
    - east → market
- NPCs:
    - a mercenary captain (recruiter)

### The Outskirts (`wilderness` · weather: temperate)

#### Cave Mouth `cave-mouth`

- Terrain: cave
- Exits:
    - north → forest-edge
    - east → foothills
    - down → old-mine

#### Deep Forest `deep-forest`

- Terrain: forest
- Exits:
    - south → foothills
    - west → forest-edge

#### Rocky Foothills `foothills`

- Terrain: mountain
- Exits:
    - north → deep-forest
    - west → cave-mouth

#### The Forest's Edge `forest-edge`

- Terrain: forest
- Notes: items present
- Exits:
    - south → cave-mouth
    - east → deep-forest
    - west → meadow

#### Long-Grass Meadow `meadow`

- Terrain: grassland
- Notes: items present
- Exits:
    - north → village-gate
    - east → forest-edge

#### The Old Diggings `old-mine`

- Terrain: cave
- Exits:
    - up → cave-mouth

#### The Village Gate `village-gate`

- Terrain: —
- Exits:
    - north → town-square (cross-area)
    - south → meadow
- NPCs:
    - Hob the stablemaster (stable)

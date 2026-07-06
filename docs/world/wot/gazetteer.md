# wot — Gazetteer

Region → area → room reference for the `wot` content pack — 109 rooms across 18 areas. Derived from the pack YAML — regenerate with `make worlddoc` or the `world-docs` skill; do not hand-edit.

## Andor

### Arien (`arien`)

#### Arien `the-arien-green`

- Terrain: grassland
- Exits:
    - north → the-arien-inn
    - south → the-arien-wright
    - east → the-wagon-road (cross-area)
    - west → the-eastward-road (cross-area)

#### Arien — the Wagoner's Rest `the-arien-inn`

- Terrain: indoors
- Exits:
    - south → the-arien-green
- NPCs:
    - the Wagoner's Rest innkeeper (shop)

#### Arien — the Wainwright's Yard `the-arien-wright`

- Terrain: outdoors
- Exits:
    - north → the-arien-green
- NPCs:
    - the Arien wainwright (shop)

### Baerlon (`baerlon`)

#### Baerlon — Market Street `market-street`

- Terrain: outdoors
- Exits:
    - north → the-stag-and-lion
    - south → the-silverworks
    - east → the-town-square
    - west → the-mountain-gate
- NPCs:
    - a Child of the Light (faction: children-of-the-light)

#### Baerlon — the Caemlyn Gate `the-caemlyn-gate`

- Terrain: outdoors
- Exits:
    - east → the-caemlyn-road (cross-area)
    - west → the-governors-hall

#### Baerlon — the Governor's Hall `the-governors-hall`

- Terrain: outdoors
- Exits:
    - east → the-caemlyn-gate
    - west → the-town-square
- NPCs:
    - the Governor of Baerlon

#### Baerlon — the Market Square `the-market-square`

- Terrain: outdoors
- Exits:
    - north → the-mining-quarter
    - south → the-town-square
- NPCs:
    - a general dealer (shop)

#### Baerlon — the Mining Quarter `the-mining-quarter`

- Terrain: mountain
- Exits:
    - south → the-market-square
- NPCs:
    - a soot-streaked miner

#### Baerlon — the Mountain Gate `the-mountain-gate`

- Terrain: outdoors
- Exits:
    - east → market-street
    - west → baerlon-approach (cross-area)
- NPCs:
    - a Baerlon gate guard (faction: queens-guard)

#### Baerlon — the Silverworks `the-silverworks`

- Terrain: outdoors
- Notes: craft station
- Exits:
    - north → market-street
- NPCs:
    - the Baerlon silversmith (shop, trainer)

#### Baerlon — the Stag and Lion `the-stag-and-lion`

- Terrain: indoors
- Exits:
    - south → market-street
- NPCs:
    - Master Fitch (shop)

#### Baerlon — the Town Square `the-town-square`

- Terrain: outdoors
- Exits:
    - north → the-market-square
    - east → the-governors-hall
    - west → market-street

### Caemlyn (`caemlyn`)

#### Caemlyn — the Great Square `the-caemlyn-square`

- Terrain: outdoors
- Exits:
    - north → the-queens-blessing
    - south → the-full-moon-street
    - east → the-sunrise-gate
    - west → the-new-city

#### Caemlyn — the Far Madding Gate `the-far-madding-gate`

- Terrain: outdoors
- Exits:
    - north → the-full-moon-street

#### Caemlyn — Full Moon Street `the-full-moon-street`

- Terrain: outdoors
- Exits:
    - north → the-caemlyn-square
    - south → the-far-madding-gate
    - west → the-mondel-gate

#### Caemlyn — the Inner City `the-inner-city`

- Terrain: outdoors
- Exits:
    - north → the-waygate
    - south → the-queens-plaza
    - east → the-mondel-gate

#### Caemlyn — the Lugard Gate `the-lugard-gate`

- Terrain: outdoors
- Exits:
    - north → the-new-city

#### Caemlyn — the Mondel Gate `the-mondel-gate`

- Terrain: outdoors
- Exits:
    - east → the-full-moon-street
    - west → the-inner-city

#### Caemlyn — the New City `the-new-city`

- Terrain: outdoors
- Exits:
    - north → the-tar-valon-gate
    - south → the-lugard-gate
    - east → the-caemlyn-square
    - west → the-whitebridge-gate
- NPCs:
    - a city dealer (shop)

#### Caemlyn — the Queen's Blessing `the-queens-blessing`

- Terrain: indoors
- Exits:
    - south → the-caemlyn-square
- NPCs:
    - Basel Gill (shop)

#### Caemlyn — the Queen's Plaza `the-queens-plaza`

- Terrain: outdoors
- Exits:
    - north → the-inner-city
    - south → the-royal-palace

#### Caemlyn — the Royal Palace `the-royal-palace`

- Terrain: outdoors
- Exits:
    - north → the-queens-plaza
- NPCs:
    - a Palace Guardsman (quest giver, faction: queens-guard)

#### Caemlyn — the Sunrise Gate `the-sunrise-gate`

- Terrain: outdoors
- Exits:
    - west → the-caemlyn-square

#### Caemlyn — the Tar Valon Gate `the-tar-valon-gate`

- Terrain: outdoors
- Exits:
    - south → the-new-city

#### Caemlyn — the Waygate `the-waygate`

- Terrain: outdoors
- Exits:
    - south → the-inner-city

#### Caemlyn — the Whitebridge Gate `the-whitebridge-gate`

- Terrain: outdoors
- Exits:
    - east → the-new-city
    - west → the-caemlyn-approach (cross-area)
- NPCs:
    - a Queen's Guardsman (faction: queens-guard)

### Carysford (`carysford`)

#### The East Bank of the Cary `the-cary-bank`

- Terrain: grassland
- Exits:
    - east → the-caemlyn-approach (cross-area)
    - west → the-ford

#### Carysford `the-carysford-green`

- Terrain: outdoors
- Exits:
    - north → the-carysford-inn
    - east → the-ford
    - west → the-carysford-road (cross-area)

#### Carysford — the Leaping Fish `the-carysford-inn`

- Terrain: indoors
- Exits:
    - south → the-carysford-green
- NPCs:
    - the Leaping Fish innkeeper (shop)

#### The Ford of the Cary `the-ford`

- Terrain: outdoors
- Exits:
    - east → the-cary-bank
    - west → the-carysford-green
- NPCs:
    - a weathered fisherman

### Comfrey (`comfrey`)

#### Comfrey `comfrey-green`

- Terrain: mountain
- Exits:
    - north → comfrey-inn
    - south → the-mist-road (cross-area)
    - west → the-mist-passes

#### Comfrey — the Miner's Rest `comfrey-inn`

- Terrain: indoors
- Exits:
    - south → comfrey-green
- NPCs:
    - the Miner's Rest innkeeper (shop)

#### The High Passes of the Mountains of Mist `the-mist-passes`

- Terrain: mountain
- Exits:
    - east → comfrey-green

### Four Kings (`four-kings`)

#### Four Kings — the West End `four-kings-gate`

- Terrain: outdoors
- Exits:
    - east → the-kings-crossroads
    - west → the-four-kings-approach (cross-area)
- NPCs:
    - a hard-faced tough

#### Four Kings — the Dancing Cartman `the-dancing-cartman`

- Terrain: indoors
- Exits:
    - south → the-kings-crossroads
- NPCs:
    - Saml Hake (shop)

#### Four Kings — the East End `the-east-end`

- Terrain: outdoors
- Exits:
    - east → the-sheran-road (cross-area)
    - west → the-kings-crossroads

#### Four Kings — the Crossroads `the-kings-crossroads`

- Terrain: outdoors
- Exits:
    - north → the-dancing-cartman
    - south → the-wagon-yard
    - east → the-east-end
    - west → four-kings-gate

#### Four Kings — the Wagon Yard `the-wagon-yard`

- Terrain: outdoors
- Exits:
    - north → the-kings-crossroads
- NPCs:
    - a well-dressed merchant (shop, faction: darkfriends)

### Market Sheran (`market-sheran`)

#### Market Sheran — the East Road `the-sheran-east`

- Terrain: grassland
- Exits:
    - east → the-carysford-road (cross-area)
    - west → the-sheran-green

#### Market Sheran — the Green `the-sheran-green`

- Terrain: outdoors
- Exits:
    - north → the-sheran-market
    - south → the-sheran-inn
    - east → the-sheran-east
    - west → the-sheran-road (cross-area)

#### Market Sheran — the Harvest Home `the-sheran-inn`

- Terrain: indoors
- Exits:
    - north → the-sheran-green
- NPCs:
    - the Harvest Home innkeeper (shop)

#### Market Sheran — the Market `the-sheran-market`

- Terrain: outdoors
- Exits:
    - south → the-sheran-green
- NPCs:
    - a market stallholder (shop)

### The Baerlon Road (`baerlon-road`)

#### The Mountain Gate Road `baerlon-approach`

- Terrain: mountain
- Exits:
    - north → the-comfrey-road
    - south → the-andor-road
    - east → the-mountain-gate (cross-area)

#### The Andor Road `the-andor-road`

- Terrain: grassland
- Exits:
    - north → baerlon-approach
    - south → the-north-bank

#### The Comfrey Road `the-comfrey-road`

- Terrain: mountain
- Exits:
    - north → the-mist-road
    - south → baerlon-approach

#### The Skirts of the Mountains of Mist `the-mist-road`

- Terrain: mountain
- Exits:
    - north → comfrey-green (cross-area)
    - south → the-comfrey-road

#### The North Bank of the Taren `the-north-bank`

- Terrain: grassland
- Exits:
    - north → the-andor-road
    - south → the-ferry-landing (cross-area)

### The Caemlyn Road (`caemlyn-road`)

#### The Arinelle Vale `the-arinelle-vale`

- Terrain: grassland
- Exits:
    - east → the-west-bank
    - west → the-caemlyn-road

#### The Approach to Caemlyn `the-caemlyn-approach`

- Terrain: grassland
- Exits:
    - east → the-whitebridge-gate (cross-area)
    - west → the-cary-bank (cross-area)

#### The Caemlyn Road `the-caemlyn-road`

- Terrain: grassland
- Exits:
    - east → the-arinelle-vale
    - west → the-caemlyn-gate (cross-area)

#### The Caemlyn Road — toward Carysford `the-carysford-road`

- Terrain: grassland
- Exits:
    - east → the-carysford-green (cross-area)
    - west → the-sheran-east (cross-area)

#### The Caemlyn Road — East of Whitebridge `the-eastward-road`

- Terrain: grassland
- Exits:
    - east → the-arien-green (cross-area)
    - west → whitebridge-square (cross-area)

#### The Approach to Four Kings `the-four-kings-approach`

- Terrain: grassland
- Exits:
    - east → four-kings-gate (cross-area)
    - west → the-wagon-road

#### The Caemlyn Road — toward Market Sheran `the-sheran-road`

- Terrain: grassland
- Exits:
    - east → the-sheran-green (cross-area)
    - west → the-east-end (cross-area)

#### The Caemlyn Road — the Wagon Road `the-wagon-road`

- Terrain: grassland
- Exits:
    - east → the-four-kings-approach
    - west → the-arien-green (cross-area)

#### The West Bank of the Arinelle `the-west-bank`

- Terrain: grassland
- Exits:
    - east → the-white-bridge (cross-area)
    - west → the-arinelle-vale

### Whitebridge (`whitebridge`)

#### Whitebridge — the Bridge Foot `the-bridge-foot`

- Terrain: outdoors
- Exits:
    - east → whitebridge-square
    - west → the-white-bridge
- NPCs:
    - a Queen's Guardsman (faction: queens-guard)

#### Whitebridge — the Wayfarers' Rest `the-wayfarers-rest`

- Terrain: indoors
- Exits:
    - south → whitebridge-square
- NPCs:
    - Master Bartim (shop)

#### The White Bridge `the-white-bridge`

- Terrain: outdoors
- Exits:
    - east → the-bridge-foot
    - west → the-west-bank (cross-area)

#### Whitebridge — the River Docks `the-whitebridge-docks`

- Terrain: outdoors
- Exits:
    - north → whitebridge-square
- NPCs:
    - a weathered riverman

#### Whitebridge — the Market Square `whitebridge-square`

- Terrain: outdoors
- Exits:
    - north → the-wayfarers-rest
    - south → the-whitebridge-docks
    - east → the-eastward-road (cross-area)
    - west → the-bridge-foot
- NPCs:
    - a sharp-tongued dealer (shop)

## Two Rivers

### Deven Ride (`deven-ride`)

#### The Sheepfold `deven-ride-fold`

- Terrain: outdoors
- Exits:
    - east → the-fen-edge (cross-area)
    - west → deven-ride-green
- NPCs:
    - a Deven Ride shepherd

#### Deven Ride — the Green `deven-ride-green`

- Terrain: outdoors
- Exits:
    - north → the-south-pasture
    - east → deven-ride-fold
- NPCs:
    - a Deven Ride weaver

#### The South Pasture `the-south-pasture`

- Terrain: grassland
- Exits:
    - north → old-road (cross-area)
    - south → deven-ride-green

### Emond's Field (`emonds-field` · weather: two-rivers)

#### The Winespring Inn — Common Room `inn-common-room`

- Terrain: indoors
- Exits:
    - north → inn-kitchen
    - east → the-inn-storeroom (locked door: a stout storeroom door)
    - west → the-green
    - up → inn-guestroom
- NPCs:
    - Bran al'Vere
    - Loial son of Arent son of Halan
    - a caravan guard-captain (recruiter)
    - a road-worn guard (hireling)

#### The Winespring Inn — Guest Room `inn-guestroom`

- Terrain: indoors
- Exits:
    - down → inn-common-room

#### The Winespring Inn — Kitchen `inn-kitchen`

- Terrain: indoors
- Notes: craft station, items present
- Exits:
    - south → inn-common-room
- NPCs:
    - Marin al'Vere (shop, trainer)

#### Jon Thane's Mill `jon-thanes-mill`

- Terrain: outdoors
- Exits:
    - east → the-aybara-farm
    - west → the-wagon-bridge
- NPCs:
    - Jon Thane

#### The North Road `north-road`

- Terrain: outdoors
- Exits:
    - north → the-north-pasture (cross-area)
    - south → the-green
    - east → the-wagon-bridge

#### The Old Road `old-road`

- Terrain: outdoors
- Exits:
    - north → the-winespring
    - south → the-south-pasture (cross-area)

#### The Quarry Road `quarry-road`

- Terrain: outdoors
- Exits:
    - north → wisdoms-cottage
    - south → the-althor-farm
    - east → the-green
    - west → the-forge
- NPCs:
    - a brigand archer (hostile)
    - a brigand cutthroat (hostile)

#### The al'Thor Farm `the-althor-farm`

- Terrain: outdoors
- Exits:
    - north → quarry-road
- NPCs:
    - Aldin the ostler (stable)

#### The Aybara Farm `the-aybara-farm`

- Terrain: outdoors
- Exits:
    - west → jon-thanes-mill

#### The Smithy `the-forge`

- Terrain: indoors
- Notes: craft station, items present
- Exits:
    - east → quarry-road
    - west → westwood-edge (cross-area)
- NPCs:
    - Haral Luhhan (shop, trainer)

#### The Green `the-green`

- Terrain: outdoors
- Notes: start room
- Exits:
    - north → north-road
    - south → the-winespring
    - east → inn-common-room
    - west → quarry-road
- NPCs:
    - Cenn Buie

#### The Winespring Inn — Storeroom `the-inn-storeroom`

- Terrain: indoors
- Notes: items present
- Exits:
    - west → inn-common-room (locked door: a stout storeroom door)

#### Emond's Field — the Sickhouse `the-sickhouse`

- Terrain: indoors
- Exits:
    - west → wisdoms-cottage

#### The Wagon Bridge `the-wagon-bridge`

- Terrain: outdoors
- Exits:
    - east → jon-thanes-mill
    - west → north-road

#### The Winespring `the-winespring`

- Terrain: outdoors
- Exits:
    - north → the-green
    - south → old-road

#### The Wisdom's Cottage `wisdoms-cottage`

- Terrain: herb-garden
- Exits:
    - south → quarry-road
    - east → the-sickhouse
- NPCs:
    - Nynaeve al'Meara

### Stedding Chinden (`stedding-chinden`)

#### The Mounded Dwellings `chinden-dwellings`

- Terrain: field
- Exits:
    - south → chinden-stump
- NPCs:
    - an Ogier

#### The Gardens of Chinden `chinden-gardens`

- Terrain: field
- Exits:
    - north → chinden-stump
- NPCs:
    - an Ogier

#### The Treesong Grove `chinden-grove`

- Terrain: forest
- Exits:
    - north → chinden-waygate
    - east → chinden-stump
- NPCs:
    - an Ogier Treesinger

#### The Stump of Stedding Chinden `chinden-stump`

- Terrain: field
- Exits:
    - north → chinden-dwellings
    - south → chinden-gardens
    - east → the-elders-stone (cross-area)
    - west → chinden-grove
- NPCs:
    - the Eldest of Stedding Chinden

#### The Overgrown Waygate `chinden-waygate`

- Terrain: forest
- Exits:
    - south → chinden-grove

### Taren Ferry (`taren-ferry`)

#### Taren Ferry `taren-ferry`

- Terrain: outdoors
- Exits:
    - north → the-ferry-landing
    - south → taren-road
- NPCs:
    - a sharp-faced villager

#### The Taren Road `taren-road`

- Terrain: grassland
- Exits:
    - north → taren-ferry
    - south → watch-hill-lookout (cross-area)

#### The Ferry Landing `the-ferry-landing`

- Terrain: outdoors
- Exits:
    - north → the-north-bank (cross-area)
    - south → taren-ferry
- NPCs:
    - the Taren ferryman

### The Mountains of Mist (`mountains-of-mist`)

#### The Deep Diggings `deep-diggings`

- Terrain: cave
- Notes: items present
- Exits:
    - up → the-diggings
    - down → the-sealed-drift (locked door: an iron door)

#### The Sand Hills `sand-hills-foot`

- Terrain: mountain
- Exits:
    - east → deep-westwood (cross-area)
    - west → the-diggings
    - up → the-mist-trail

#### The Cloudgate `the-cloudgate`

- Terrain: mountain
- Exits:
    - west → the-vale-of-chinden
    - down → the-mist-trail

#### The Old Diggings `the-diggings`

- Terrain: cave
- Notes: items present
- Exits:
    - east → sand-hills-foot
    - west → the-hidden-gallery (hidden)
    - down → deep-diggings

#### The Elder's Stone `the-elders-stone`

- Terrain: field
- Exits:
    - east → the-vale-of-chinden
    - west → chinden-stump (cross-area)

#### A Hidden Gallery `the-hidden-gallery`

- Terrain: cave
- Notes: items present
- Exits:
    - east → the-diggings

#### The Mist Trail `the-mist-trail`

- Terrain: mountain
- Exits:
    - up → the-cloudgate
    - down → sand-hills-foot

#### The Sealed Drift `the-sealed-drift`

- Terrain: cave
- Notes: items present
- Exits:
    - up → deep-diggings (locked door: an iron door)

#### The Vale of Chinden `the-vale-of-chinden`

- Terrain: field
- Exits:
    - east → the-cloudgate
    - west → the-elders-stone

### The Waterwood (`the-waterwood`)

#### The Black Pools `the-black-pools`

- Terrain: swamp
- Exits:
    - north → the-reed-beds
- NPCs:
    - a writhing knot of leeches (hostile)

#### The Drowned Wood `the-drowned-wood`

- Terrain: swamp
- Exits:
    - east → the-mire-heart
    - west → the-reed-beds
- NPCs:
    - a marsh adder

#### The Fen Edge `the-fen-edge`

- Terrain: swamp
- Exits:
    - east → the-reed-beds
    - west → deven-ride-fold (cross-area)
- NPCs:
    - a fen-trapper

#### The Heart of the Mire `the-mire-heart`

- Terrain: swamp
- Exits:
    - south → the-white-river-bank
    - west → the-drowned-wood

#### The Reed-Beds `the-reed-beds`

- Terrain: swamp
- Exits:
    - south → the-black-pools
    - east → the-drowned-wood
    - west → the-fen-edge

#### The White River Bank `the-white-river-bank`

- Terrain: swamp
- Exits:
    - north → the-mire-heart

### The Westwood (`westwood`)

#### Deep in the Westwood `deep-westwood`

- Terrain: forest
- Exits:
    - east → westwood-edge
    - west → sand-hills-foot (cross-area)

#### The Edge of the Westwood `westwood-edge`

- Terrain: forest
- Notes: items present
- Exits:
    - east → the-forge (cross-area)
    - west → deep-westwood

### Watch Hill (`watch-hill`)

#### The North Pasture `the-north-pasture`

- Terrain: grassland
- Exits:
    - north → watch-hill-green
    - south → north-road (cross-area)

#### Watch Hill — the Green `watch-hill-green`

- Terrain: outdoors
- Exits:
    - north → watch-hill-lookout
    - south → the-north-pasture
    - east → watch-hill-inn

#### Watch Hill — the Goose and Crown `watch-hill-inn`

- Terrain: indoors
- Exits:
    - west → watch-hill-green
- NPCs:
    - the Goose and Crown's innkeeper

#### Watch Hill — the Lookout `watch-hill-lookout`

- Terrain: outdoors
- Exits:
    - north → taren-road (cross-area)
    - south → watch-hill-green
- NPCs:
    - an old watchman

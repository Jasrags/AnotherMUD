-- track_kills.lua — M17.1c Theme D demo target.
--
-- Subscribes to mob.killed and logs every kill through the engine
-- logger. Demonstrates the minimal scripting surface: subscribe
-- to a bus event by name, receive the payload as a Lua table with
-- snake_case field names, and emit a log line via engine.log.
--
-- Payload schema (eventbus.MobKilled, snake_case-translated):
--   mob_id        — runtime entity id of the dead mob
--   mob_name      — display name of the dead mob
--   template_id   — content template id (mob species)
--   killer_id     — combatant id of the killer (player or mob)
--   killer_name   — display name of the killer
--   room_id       — namespaced room id where the kill happened

engine.subscribe("mob.killed", function(name, p)
  engine.log("kill: " .. tostring(p.killer_name) ..
             " killed " .. tostring(p.mob_name) ..
             " (" .. tostring(p.template_id) .. ")" ..
             " in " .. tostring(p.room_id))
end)

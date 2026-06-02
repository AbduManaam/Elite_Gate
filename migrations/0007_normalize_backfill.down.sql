
DELETE FROM route_methods;

UPDATE routes SET policy_id = NULL;

DELETE FROM policies;

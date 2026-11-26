-- From 2026-11-23 the catalogue writes a filament diameter in millimetres,
-- as 1.75, and release 0.2.8 read it as whole micrometres: 1.75 became 1. No
-- filament is under 100 micrometres, so such a value is one of those. The
-- fraction is lost, so the whole millimetre stands for the diameter sold:
-- 1 for 1.75 mm, 2 for 2.85 mm, 3 for 3 mm. Anything else is left unknown,
-- which counts as 1.75 mm. A spool copied its package's diameter when added.
UPDATE packages SET diameter_um = CASE diameter_um WHEN 1 THEN 1750 WHEN 2 THEN 2850 WHEN 3 THEN 3000 END
    WHERE diameter_um < 100;
UPDATE spools SET diameter_um = CASE diameter_um WHEN 1 THEN 1750 WHEN 2 THEN 2850 WHEN 3 THEN 3000 END
    WHERE diameter_um < 100;

ALTER TABLE checks RENAME TO reviews; 
ALTER TABLE reviews RENAME COLUMN check_id TO review_id;
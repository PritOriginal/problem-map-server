ALTER TABLE reviews RENAME TO checks; 
ALTER TABLE checks RENAME COLUMN review_id TO check_id;
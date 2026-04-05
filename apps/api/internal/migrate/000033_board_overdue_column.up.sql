-- Add an "Overdue" column to every board that does not already have one (name match, case-insensitive).
-- New boards get this column from CreateBoardWithDefaultColumns (see board.go).

INSERT INTO board_columns (board_id, name, position)
SELECT b.id,
       'Overdue',
       COALESCE(mx.max_pos, -1) + 1
FROM boards b
LEFT JOIN (
  SELECT board_id, MAX(position) AS max_pos
  FROM board_columns
  GROUP BY board_id
) mx ON mx.board_id = b.id
WHERE NOT EXISTS (
  SELECT 1
  FROM board_columns c
  WHERE c.board_id = b.id AND lower(trim(c.name)) = 'overdue'
);

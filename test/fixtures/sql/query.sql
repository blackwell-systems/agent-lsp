-- Get all persons with their posts
SELECT
  p.id,
  p.name,
  p.age,
  po.title
FROM person p
JOIN post po ON po.author_id = p.id
WHERE p.age > 18;

-- Count posts per person
SELECT
  p.name,
  COUNT(po.id) AS post_count
FROM person p
LEFT JOIN post po ON po.author_id = p.id
GROUP BY p.name
ORDER BY post_count DESC;

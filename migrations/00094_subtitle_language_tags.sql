-- +goose Up
SET LOCAL lock_timeout='5s';
SET LOCAL statement_timeout='60s';
ALTER TABLE managed_subtitles ADD COLUMN language_tags text[] NOT NULL DEFAULT '{}';
ALTER TABLE managed_subtitles ADD CONSTRAINT managed_subtitle_language_tags_bound CHECK(cardinality(language_tags)<=8 AND array_to_string(language_tags,',') !~ '[[:cntrl:]]');
-- Only names and existing metadata are inspected. No provider/content request.
WITH names AS (
 SELECT id,lower(filename) name,lower(language) old,
 lower(filename) ~ '(^|[^a-z0-9])(chs|sc|zh-hans|zh-cn|gb|gbk|gb18030)([^a-z0-9]|$)' hans,
 lower(filename) ~ '(^|[^a-z0-9])(cht|tc|zh-hant|zh-tw|big5)([^a-z0-9]|$)' hant,
 lower(filename) ~ '(^|[^a-z0-9])(en|eng)([^a-z0-9]|$)' english,
 lower(filename) ~ '(^|[^a-z0-9])(zh|chi|zho)([^a-z0-9]|$)' chinese,
 lower(filename) ~ '(^|[^a-z0-9])(ja|jp|jpn)([^a-z0-9]|$)' japanese,
 lower(filename) ~ '(^|[^a-z0-9])(ko|kor)([^a-z0-9]|$)' korean
 FROM managed_subtitles
), inferred AS (
 SELECT *,CASE WHEN ((hans OR hant OR chinese) AND english) OR name LIKE '%简英%' OR name LIKE '%繁英%' OR name LIKE '%中英%' THEN 'chi'
 WHEN hans OR hant OR chinese THEN 'chi' WHEN english THEN 'eng' WHEN japanese THEN 'jpn' WHEN korean THEN 'kor' ELSE 'und' END detected,
 CASE WHEN old IN ('chs','cht','zho','zh','chi','zh-cn','zh-tw','zh-hans','zh-hant') THEN 'chi' WHEN old IN ('en','eng') THEN 'eng' WHEN old IN ('ja','jp','jpn') THEN 'jpn' WHEN old IN ('ko','kor') THEN 'kor' WHEN old IN ('fra','fr') THEN 'fre' WHEN old IN ('deu','de') THEN 'ger' WHEN old='es' THEN 'spa' WHEN old='ru' THEN 'rus' WHEN old='it' THEN 'ita' WHEN old='pt' THEN 'por' WHEN old='ar' THEN 'ara' WHEN old ~ '^[a-z]{3}$' THEN old ELSE 'und' END canonical
 FROM names
), final AS (
 SELECT *,CASE WHEN canonical='und' THEN detected ELSE canonical END primary_language FROM inferred
)
UPDATE managed_subtitles s SET language=f.primary_language,language_tags=CASE
 WHEN f.primary_language='chi' THEN
  CASE WHEN f.hant OR f.old IN ('cht','zh-tw','zh-hant') OR f.name LIKE '%繁英%' THEN ARRAY['zh-Hant']::text[] WHEN f.hans OR f.old IN ('chs','zh-cn','zh-hans') OR f.name LIKE '%简英%' OR f.name LIKE '%中英%' THEN ARRAY['zh-Hans']::text[] ELSE '{}'::text[] END
  || CASE WHEN (f.detected='chi' AND f.english) OR f.name LIKE '%简英%' OR f.name LIKE '%繁英%' OR f.name LIKE '%中英%' THEN ARRAY['en']::text[] ELSE '{}'::text[] END
 WHEN f.primary_language='eng' THEN ARRAY['en']::text[] WHEN f.primary_language='jpn' THEN ARRAY['ja']::text[] WHEN f.primary_language='kor' THEN ARRAY['ko']::text[] ELSE '{}'::text[] END
FROM final f WHERE s.id=f.id;
-- +goose Down
ALTER TABLE managed_subtitles DROP CONSTRAINT managed_subtitle_language_tags_bound;
ALTER TABLE managed_subtitles DROP COLUMN language_tags;
-- Language aliases are intentionally not recreated: the canonical codes are valid in schema 93.

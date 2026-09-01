-- +goose Up
insert or ignore into media_file_tags (media_file_id, tag_id)
select mf.id, jt.value
from media_file mf, json_tree(mf.tags, '$.mood') jt
where jt.key = 'id' and jt.atom is not null;

insert or ignore into media_file_tags (media_file_id, tag_id)
select mf.id, jt.value
from media_file mf, json_tree(mf.tags, '$.releasetype') jt
where jt.key = 'id' and jt.atom is not null;

insert or ignore into album_tags (album_id, tag_id)
select al.id, jt.value
from album al, json_tree(al.tags, '$.mood') jt
where jt.key = 'id' and jt.atom is not null;

insert or ignore into album_tags (album_id, tag_id)
select al.id, jt.value
from album al, json_tree(al.tags, '$.releasetype') jt
where jt.key = 'id' and jt.atom is not null;

-- +goose Down
delete from media_file_tags
where tag_id in (select id from tag where tag_name in ('mood', 'releasetype'));

delete from album_tags
where tag_id in (select id from tag where tag_name in ('mood', 'releasetype'));

CREATE TRIGGER thanksgiving_toggle AFTER UPDATE OF active ON gmseasons
WHEN lower(NEW.name)='thanksgiving' AND NEW.active<>OLD.active
BEGIN
 UPDATE quests SET enabled=NEW.active WHERE source='thanksgiving-2026' AND questid BETWEEN 109910 AND 109912;
 UPDATE gmtriggers SET enabled=NEW.active WHERE slot=340 AND notes='thanksgiving-2026';
 DELETE FROM quickslots WHERE NEW.active=0 AND itemid IN (SELECT itemid FROM charinventory WHERE patternid=1000810);
 DELETE FROM charinventory WHERE NEW.active=0 AND patternid=1000810;
END;

CREATE TRIGGER thanksgiving_armor_pickup BEFORE INSERT ON charinventory
WHEN NEW.patternid=1000810 AND NOT EXISTS(SELECT 1 FROM gmseasons WHERE name='Thanksgiving' COLLATE NOCASE AND active=1)
BEGIN
 SELECT RAISE(ABORT,'Thanksgiving armor has expired');
END;

CREATE TRIGGER thanksgiving_armor_stack BEFORE UPDATE OF patternid,stackleft ON charinventory
WHEN NEW.patternid=1000810 AND NOT EXISTS(SELECT 1 FROM gmseasons WHERE name='Thanksgiving' COLLATE NOCASE AND active=1)
BEGIN
 SELECT RAISE(ABORT,'Thanksgiving armor has expired');
END;

CREATE TRIGGER thanksgiving_tribute AFTER UPDATE OF step ON charquests
WHEN NEW.questid=109910 AND OLD.step=1 AND NEW.step=2 AND NEW.done=OLD.done
BEGIN
 SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM gmseasons WHERE name='Thanksgiving' COLLATE NOCASE AND active=1)
   THEN RAISE(ABORT,'Thanksgiving muster closed') END;
 UPDATE charmarks SET marks=marks-35 WHERE serverid=NEW.serverid AND marks>=35;
 SELECT CASE WHEN changes()<>1 THEN RAISE(ABORT,'35 Heirloom Marks required') END;
 INSERT INTO charmarks_log(serverid,at,source,amount,ref,bonus)
 VALUES(NEW.serverid,strftime('%Y-%m-%dT%H:%M:%SZ','now'),'spend',-35,'Thanksgiving tribute',1);
END;

CREATE VIEW thanksgiving_open_patrol AS
 SELECT q.serverid FROM charquests q LEFT JOIN charmarks m ON m.serverid=q.serverid
 WHERE q.questid=109910 AND q.step=1 AND COALESCE(m.marks,0)<35
 AND EXISTS(SELECT 1 FROM gmseasons WHERE name='Thanksgiving' COLLATE NOCASE AND active=1)
 AND (SELECT count(*) FROM charmarks_log l WHERE l.serverid=q.serverid AND l.source='quest'
      AND l.ref IN ('Thanksgiving field patrol 109911','Thanksgiving field patrol 109912'))<5;

CREATE TRIGGER thanksgiving_field_pay AFTER UPDATE OF done ON charquests
WHEN NEW.questid IN (109911,109912) AND OLD.step=2 AND NEW.step=0 AND NEW.done=OLD.done+1
AND EXISTS(SELECT 1 FROM thanksgiving_open_patrol WHERE serverid=NEW.serverid)
BEGIN
 INSERT INTO charmarks(serverid,marks,lifetime) VALUES(NEW.serverid,7,7)
 ON CONFLICT(serverid) DO UPDATE SET marks=marks+7,lifetime=lifetime+7;
 INSERT INTO charmarks_log(serverid,at,source,amount,ref,bonus)
 VALUES(NEW.serverid,strftime('%Y-%m-%dT%H:%M:%SZ','now'),'quest',7,'Thanksgiving field patrol '||NEW.questid,1);
END;

CREATE TRIGGER thanksgiving_reward AFTER UPDATE OF done ON charquests
WHEN NEW.questid=109910 AND OLD.step=6 AND NEW.step=0 AND NEW.done=1 AND OLD.done=0
BEGIN
 SELECT CASE WHEN NOT EXISTS(SELECT 1 FROM gmseasons WHERE name='Thanksgiving' COLLATE NOCASE AND active=1)
 THEN RAISE(ABORT,'Thanksgiving muster closed') END;
 SELECT CASE WHEN EXISTS(SELECT 1 FROM charinventory WHERE serverid=NEW.serverid AND patternid=1000812)
 THEN RAISE(ABORT,'You already have Turkey Smasher') END;
 INSERT INTO charinventory(serverid,stackleft,remainHP,remaincharge,patternid,equiploc,location,listnumber)
 SELECT NEW.serverid,1,maxhp,0,1000812,-1,0,COALESCE((SELECT max(listnumber)+1 FROM charinventory
 WHERE serverid=NEW.serverid AND location=0),0) FROM itempattern WHERE patternid=1000812;
 SELECT CASE WHEN changes()<>1 THEN RAISE(ABORT,'Missing Turkey Smasher') END;
END;

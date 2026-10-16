EVERQUEST ONLINE ADVENTURES -- FILES FOR TESTERS
================================================

Three programs. Your server operator will normally tell you to run one of
them; this explains what each is for and, at the end, what to send back when
something does not work.

They are PlayStation 2 programs, not Windows programs. Run them the same way
you run the game: on PCSX2 with System -> Boot ELF, or on a real PlayStation 2
with FreeMcBoot, wLaunchELF or a USB stick.


WHAT YOU NEED FIRST
-------------------

YOUR OWN COPY OF THE GAME DISC, as an unmodified ISO. Make it from your own
disc. Nobody here can give you one.

It must be the ORIGINAL, UNPATCHED disc image. If you have been sent a
"patched ISO" by someone, or made one yourself with an older set of
instructions, do not use it here -- EQOABOOT.ELF exists precisely so that no
patched disc is needed. It patches the game in memory every time you play, and
your copy stays original.

The right disc is the USA release of "EverQuest Online Adventures: Frontiers".
EQOABOOT checks it and will tell you if the disc is not the one it expects,
rather than starting something it does not understand.


THE THREE PROGRAMS
------------------

  EQOABOOT.ELF     Start the game.

      This is the one you use to play. It reads the game off your own disc,
      applies the settings for this server in memory, and starts it. Your disc
      is never written to.

      It already knows this server's address -- the server generated this copy
      for itself, so it cannot be pointed at the wrong machine. The address is
      printed on screen when it starts; that is the server you are connecting
      to.

      On PCSX2 you may start it with NO disc loaded at all. It will say which
      ISO to load and wait for you. Load it and it carries on by itself --
      nothing needs restarting.

  EQOAHDD.ELF      Prepare a PlayStation 2 hard disk, and repair the memory
                   card.

      Two jobs. It sets up a hard disk to hold the extra worlds this server
      streams, which you only ever do once, and which you do not need at all
      unless you are using a hard disk.

      It can also write the game's own files onto your memory card, and delete
      them so the game downloads them again. Use those if you are told to, or
      if the game misbehaves in a way that sounds like a bad file. Its own
      README, EQOAHDD-README.txt, covers all of it.

  EQOADIAG.ELF     Find out why you cannot connect.

      Run this when something is wrong and it is not obvious which end the
      problem is on. It checks your memory card, the address it holds, and
      every one of this server's ports using the real protocol, from the
      machine that has the problem. It writes nothing and changes nothing.

      Its report is the single most useful thing you can send back.


IN WHAT ORDER
-------------

Just playing, normal memory card:      EQOABOOT.ELF. That is all.
Using a PlayStation 2 hard disk:       EQOAHDD.ELF once, then EQOABOOT.ELF.
Something is broken:                   EQOADIAG.ELF, and send the report.


IF SOMETHING GOES WRONG
-----------------------

Send these, and it will usually be enough to say what happened without a
second round of questions:

  1. What you ran, and what you expected instead.
  2. A photo or screenshot of the screen, including any line in capitals.
     Every one of these programs prints what it did in a log at the bottom of
     the screen, and that log is the answer far more often than a description
     of it is.
  3. EQOABOOT prints a line beginning "If you need to report a problem, quote
     this line" -- quote it. It carries the server address, your disc's
     identity, and how many patches were applied.
  4. If you can get that far, the output of EQOADIAG.ELF.

Do not send us a copy of the game.


THE SMALL PRINT
---------------

EQOAHDD.ELF and EQOADIAG.ELF contain PlayStation 2 driver modules built from
ps2sdk (the ps2dev project), used under the Academic Free License 2.0. The
licence text ships beside this file as LICENSE.ps2sdk.

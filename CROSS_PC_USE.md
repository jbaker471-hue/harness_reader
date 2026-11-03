# Cross-PC use

The application and its development workspace have two different portability paths.

## Run the Windows application elsewhere

Download the verified portable ZIP from the repository's GitHub Releases page, extract it, verify `SHA256.txt`, and run `Trivex_Harness_Auditor_v2.8.exe`.

Drawing PDFs remain local to the PC running the executable. The automatic labor-reference database is also local to that Windows/browser profile. Export `Trivex_Local_Labor_References.json` on the first PC and import it on the second PC when those references need to move.

Never publish customer drawings, extracted drawing records, project files, or labor-reference exports as release assets.

## Continue development through ChatGPT/Codex

Use the GitHub repository as the source of truth. In ChatGPT/Codex, create or select a private Cloud environment backed by `jbaker471-hue/harness_reader`, or clone/pull the repository on the other PC and open that saved local project.

Cloud tasks have their own working files. Commit and push approved source changes so another task or computer starts from the same version. Customer drawings used for local testing are intentionally not part of the repository and must be supplied separately on the approved computer.

# App repositories count towards storage

The storage allowance counted a business's bucket and databases but not its apps' git
repositories, so images committed to git were stored without using any of the allowance.
Each accepted push now records its repository's size (post-receive, deployed or not), and that
size counts in the hourly meter, in billing, in the upload quota check and on the app's Storage
card, which shows a new Code row. `AppStorage` gains `repo_bytes`. Specs: CONTROL-PLANE.md
§6.3, §6.13, §6.15 and the storage route; DASHBOARD.md app overview; SKILL.md §9.

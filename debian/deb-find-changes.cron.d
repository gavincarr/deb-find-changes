# Find dpkg changes in the /etc tree
0 0 * * * root /usr/bin/deb-find-changes --no-warnings --exclude-from=/etc/deb-find-changes/exclude-etc -O /var/cache/deb-find-changes/etc.txt /etc

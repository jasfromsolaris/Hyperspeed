Create two files in this directory (same names as in docker-compose.provisioning-secrets.yml):

  provisioning_install_id     — single line: install id from Hyperspeed
  provisioning_install_secret — single line: install secret from Hyperspeed

Then run:

  docker compose -f docker-compose.yml -f docker-compose.provisioning-secrets.yml up -d --build

Do not commit real secret contents.

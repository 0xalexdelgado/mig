#!/usr/bin/env python
import os
import gnupg
from time import gmtime, strftime
import random
import requests
import json

def makeToken(gpghome, keyid):
    gpg = gnupg.GPG(gnupghome=gpghome)
    timestamp = strftime("%Y-%m-%dT%H:%M:%SZ", gmtime())
    nonce = str(random.randint(10000, 18446744073709551616)) + \
            str(random.randint(10000, 18446744073709551616))
    sig = gpg.sign(timestamp + ";" + nonce + "\n",
        keyid=keyid,
        detach=True, clearsign=True)
    token = timestamp + ";" + nonce + ";"
    linectr=0
    for line in iter(str(sig).splitlines()):
        linectr+=1
        if linectr < 4 or line.startswith('-') or not line:
            continue
        token += line
    return token

if __name__ == '__main__':
    token = makeToken("/home/ulfr/.gnupg",
        "E60892BB9BD89A69F759A1A0A3D652173B763E8F")
    r = requests.get("http://localhost:12345/api/v1/dashboard",
        headers={'X-PGPAUTHORIZATION': token})
    print r.text


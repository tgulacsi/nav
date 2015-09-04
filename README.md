# A Nemzeti Adó- és Vámhivatal adószám ellenőrzője

## Használat
http://nav.gov.hu/nav/adatbazisok/adatbleker/afaalanyok/afaalanyok_csoportos

	nav 88888888 99999999

vagy

	nav </tmp/adoszamok.txt

ahol az adoszamok.txt a NAV által megkövetelt formátumú: 8 jegyű törzsszámok, "\n" (LF) -el elválasztva.

## Telepítés

	go get github.com/tgulacsi/nav/cmd/nav


## Teendők

  * [X] Programozzuk le a [checksumot](http://muzso.hu/2011/10/26/adoszam-ellenorzo-osszeg-generator) és csak a hibátlan számokat küldjük a NAV-nak.
  * [X] Egy feltöltés csak 200kB lehet, daraboljuk a feltöltést.
  * [X] A darabolt feltöltéseket csináljuk párhuzamosan.

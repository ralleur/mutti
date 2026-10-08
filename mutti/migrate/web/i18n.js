// SPDX-License-Identifier: GPL-2.0-or-later
// Setup page language. German is the page's source text; for an English
// user (browser or Mac app language) static text, attributes and the texts
// app.js builds are translated here. Server messages arrive already in the
// request's language (Accept-Language).
(()=>{
const pick=()=>{for(const l of navigator.languages||[navigator.language||'']){const c=String(l).toLowerCase();if(c.startsWith('de'))return 'de';if(c.startsWith('en'))return 'en'}return 'en'};
const lang=pick();
const en={
'Willkommen bei Mutti':'Welcome to Mutti','Bei dir zu Hause.':'At your home.','DEINE MEDIEN. DEIN ZUHAUSE.':'YOUR MEDIA. YOUR HOME.',
'Mach es dir':'Make it','einfach.':'easy.',
'Neu anfangen oder deine Jellyfin-Bibliothek mitbringen.':'Start fresh or bring your Jellyfin library.','Mutti kümmert sich um den Umzug.':'Mutti takes care of the move.',
'Aus Jellyfin übernehmen':'Bring from Jellyfin','Benutzer, Bibliotheken, Favoriten und Wiedergabestand mitnehmen.':'Take users, libraries, favourites and playback progress with you.',
'Bibliothek mitbringen →':'Bring my library →','Neu einrichten':'Set up new','Deine Medienordner auswählen und ein neues Zuhause einrichten.':'Choose your media folders and set up a new home.',
'Frisch anfangen →':'Start fresh →','Ohne Mutti-Konto. Deine Daten bleiben bei dir.':'No Mutti account. Your data stays with you.','Zur Serverübersicht →':'Go to the server overview →',
'← Zurück':'← Back','WILLKOMMEN ZU HAUSE':'WELCOME HOME','Jellyfin zieht mit.':'Jellyfin moves in too.',
'Mit deinem vorhandenen Jellyfin-Administrator anmelden. Mutti erstellt die Sicherung, übernimmt deine Daten und prüft das Ergebnis.':'Sign in with your existing Jellyfin administrator. Mutti creates the backup, takes over your data and checks the result.',
'Suche Jellyfin auf diesem Rechner …':'Looking for Jellyfin on this computer …','Jellyfin-Adresse':'Jellyfin address','Administrator':'Administrator','Passwort':'Password',
'Deine bisherige Mutti-Einrichtung bleibt gesichert.':'Your existing Mutti setup stays backed up.',
'Für den Wechsel bestätige den Administratorzugang der bestehenden Bibliothek. Das ist kein zusätzliches Mutti-Konto.':'To switch, confirm the administrator account of the existing library. This is not an additional Mutti account.',
'Administrator der bestehenden Bibliothek':'Administrator of the existing library','Passwort der bestehenden Bibliothek':'Password of the existing library',
'Zur übernommenen Bibliothek wechseln':'Switch to the imported library','Medien liegen jetzt an einem anderen Ort':'My media is in a different place now',
'Die Mediendateien bleiben in deinen Ordnern. Falls nötig, ordne den bisherigen Speicherort einem erreichbaren Ordner zu.':'Your media files stay in your folders. If needed, map the previous location to a reachable folder.',
'Ordner zuordnen +':'Map a folder +',
'Mutti bereitet Jellyfin automatisch für den Umzug vor und sichert geänderte Einstellungen. Falls nötig, wird Jellyfin kurz neu gestartet; eine laufende Wiedergabe wird dabei unterbrochen.':'Mutti prepares Jellyfin for the move automatically and backs up changed settings. If needed, Jellyfin restarts briefly; playback in progress is interrupted.',
'Geprüft für Jellyfin 12.1. Der bestehende Server bleibt erhalten. Bitte während des Umzugs die Wiedergabe pausieren. Bisherige Geräte meldest du neu an. Netzwerk und Hardwarebeschleunigung werden für Mutti neu eingerichtet. Für entfernte Server werden HTTPS und der Mutti-Umzugshelfer benötigt.':'Verified for Jellyfin 12.1. The existing server is kept. Please pause playback during the move. Sign in your devices again. Network and hardware acceleration are set up again for Mutti. Remote servers need HTTPS and the Mutti moving helper.',
'Jellyfin übernehmen →':'Import Jellyfin →','MUTTI KÜMMERT SICH':'MUTTI TAKES CARE OF IT','Alles zieht mit.':'Everything moves in.','Umzugsschritte':'Moving steps',
'Kurt zieht mit.':'Kurt is helping.','Kurt pausieren':'Pause Kurt','Kurt fortsetzen':'Resume Kurt','Gesamtdauer':'Total time','In diesem Schritt':'In this step',
'Bitte lasse Mutti geöffnet. Bei großen Bibliotheken können Sicherung und Prüfung länger dauern. Deine Mediendateien bleiben an ihrem Ort.':'Please keep Mutti open. With large libraries, backup and verification can take longer. Your media files stay where they are.',
'Abbrechen beendet die Übernahme. Eine bereits gestartete Sicherung kann auf Jellyfin weiterlaufen.':'Cancelling stops the import. A backup that has already started may continue on Jellyfin.',
'Übernahme abbrechen':'Cancel import','SCHÖN, DASS DU DA BIST':'GOOD TO HAVE YOU HERE','Alles zu Hause.':'Everything is home.',
'Deine Bibliothek ist geprüft und bereit.':'Your library is verified and ready.','Dein bisheriger Jellyfin-Zugang funktioniert weiter.':'Your existing Jellyfin sign-in keeps working.',
'Serverübersicht öffnen →':'Open the server overview →','Geräte kannst du jetzt über „Geräte koppeln“ verbinden.':'You can now connect devices with “Pair devices”.',
// app.js
'Keinen eingerichteten Jellyfin-Server gefunden. Trage seine Adresse ein.':'No Jellyfin server that is set up was found. Enter its address.',
'Automatische Suche nicht verfügbar. Trage die Serveradresse ein.':'Automatic search is unavailable. Enter the server address.',
'Bisheriger Pfad':'Previous path','Bisheriger Medienpfad':'Previous media path','Ordner auf diesem Rechner':'Folder on this computer','Neuer Medienpfad':'New media path','Auswählen':'Choose','Entfernen':'Remove',
'Den Wechsel bestätigst du direkt in der App. Danach verwendest du deinen bisherigen Jellyfin-Zugang weiter.':'You confirm the switch directly in the app. Afterwards you keep using your existing Jellyfin sign-in.',
'Benutzer':'Users','Bibliotheken':'Libraries','Medieneinträge':'Media items','Wiedergabedaten':'Playback data',
'Zugang prüfen':'Check access','Jellyfin sichern':'Back up Jellyfin','Umzug vorbereiten':'Prepare the move','Daten übernehmen':'Take over data','Ergebnis prüfen':'Check the result','Quelle abgleichen':'Compare with the source','Mutti öffnen':'Open Mutti',
'Mutti ist gerade nicht erreichbar. Die Verbindung wird automatisch erneut versucht.':'Mutti is not reachable right now. The connection is retried automatically.',
'Status wird geladen …':'Loading status …','Status wird laufend aktualisiert · letzte Rückmeldung vor {0}':'Status is updated continuously · last update {0} ago',
'Verbindung zu Mutti unterbrochen. Letzte Rückmeldung vor {0} Der Vorgang kann weiterlaufen; die Anzeige versucht automatisch, sich wieder zu verbinden.':'Connection to Mutti interrupted. Last update {0} ago. The task may continue; this page reconnects automatically.',
'{0}: {1} · letzte messbare Änderung vor {2}':'{0}: {1} · last measurable change {2} ago',
'Dieser Schritt liefert noch keine weiteren Fortschrittsdaten.':'This step does not report further progress yet.',
'Für diesen Arbeitsschritt liegen noch keine messbaren Fortschrittsdaten vor.':'There is no measurable progress for this step yet.',
'Seit längerer Zeit ist keine weitere Dateiänderung messbar.':'No further file change has been measurable for a while.',
' Jellyfin kann Daten zunächst intern verarbeiten. Ein Stillstand ist damit nicht sicher nachgewiesen. ':' Jellyfin may first process data internally, so this does not prove that it is stuck. ',
'Wenn das anhält, bitte den Sicherungsstatus und die Protokolle auf Jellyfin prüfen.':'If this continues, please check the backup status and the logs on Jellyfin.',
'Wenn das anhält, bitte die Mutti-Protokolle prüfen.':'If this continues, please check the Mutti logs.',
'{0} Std. {1} Min.':'{0} h {1} min','{0} Min. {1} Sek.':'{0} min {1} s','{0} Sek.':'{0} s',
};
window.muttiLang=lang;
window.t=(text,...args)=>{const s=lang==='en'&&Object.hasOwn(en,text)?en[text]:text;return s.replace(/\{(\d)\}/g,(_,i)=>String(args[i]??''))};
if(lang!=='en')return;
const swap=s=>{const k=s.trim();return k&&Object.hasOwn(en,k)?s.replace(k,en[k]):s};
const apply=()=>{document.documentElement.lang='en';document.title=swap(document.title);
const walk=document.createTreeWalker(document.body,NodeFilter.SHOW_TEXT);for(let n=walk.nextNode();n;n=walk.nextNode())n.nodeValue=swap(n.nodeValue);
for(const el of document.querySelectorAll('[placeholder],[aria-label],[title]'))for(const a of ['placeholder','aria-label','title'])if(el.hasAttribute(a))el.setAttribute(a,swap(el.getAttribute(a)))};
if(document.readyState==='loading')document.addEventListener('DOMContentLoaded',apply);else apply();
})();

/* ---------------- Onboarding ---------------- */
// setupStepper reflète l'avancement : étape PIN puis étape Connexion.
function setupStepper(pinDone) {
  const s = el("stepper"); if (!s) return;
  s.hidden = false;
  el("step-1").className = "step " + (pinDone ? "done" : "active");
  el("step-2").className = "step " + (pinDone ? "active" : "");
}
// obShow affiche un seul panneau du tunnel et cache les autres.
function obShow(which) {
  const map = { pin: "panel-pin", provider: "panel-provider", connect: "connect-form", "2fa": "panel-2fa", github: "github-form" };
  Object.values(map).forEach((id) => { const n = el(id); if (n) n.hidden = true; });
  const node = el(map[which]); if (node) node.hidden = false;
}

async function showOnboarding() {
  hideBoot();
  el("onboarding").classList.remove("hidden");
  el("app").classList.add("hidden");

  // Onboarding « PIN-only », en tunnel séquentiel : 1) un PIN (s'il n'existe pas),
  // 2) la connexion Forgejo, 2b) le code 2FA seulement si le compte en a un. Les
  // identifiants déjà saisis ne sont jamais redemandés.
  let pinDone = await App().PinSet().catch(() => false);

  const welcome = el("welcome-card"), card = el("connect-card");
  let onOtpComplete = null;
  const ciPin = codeInput(el("f-pin"), true, null, false);
  const ciPin2 = codeInput(el("f-pin2"), true, () => el("pin-next").click());
  const ciOtp = codeInput(el("f-otp"), false, () => { if (onOtpComplete) onOtpComplete(); });

  // Étape 1 — PIN. « Continuer » valide le PIN, le définit, puis passe à la connexion.
  el("pin-next").onclick = async () => {
    const err = el("pin-err"); err.classList.add("hidden");
    const p1 = ciPin.read(), p2 = ciPin2.read();
    if (p1.length < CODELEN) { err.textContent = "Le PIN doit faire 6 chiffres."; err.classList.remove("hidden"); return; }
    if (p1 !== p2) { err.textContent = "Les PIN ne correspondent pas."; err.classList.remove("hidden"); ciPin2.clear(); return; }
    const btn = el("pin-next"); btn.disabled = true; btn.textContent = "…";
    try { await App().SetPin("", p1); pinDone = true; }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); btn.disabled = false; btn.textContent = "Continuer"; return; }
    gotoProvider();
  };

  // Étape 2 — choix du fournisseur, puis la connexion correspondante.
  function gotoProvider() { setupStepper(true); obShow("provider"); }
  function gotoConnect() {
    setupStepper(true);
    obShow("connect");
    setTimeout(() => { const fn = el("f-name"); if (fn) fn.focus(); }, 40);
  }
  function gotoGitHub() {
    setupStepper(true);
    obShow("github");
    setTimeout(() => { const fn = el("g-name"); if (fn) fn.focus(); }, 40);
  }
  document.querySelectorAll("#panel-provider [data-provider]").forEach((b) => {
    b.onclick = () => (b.dataset.provider === "github" ? gotoGitHub() : gotoConnect());
  });

  // Connexion GitHub : jeton personnel collé (SituLab ne peut pas le créer via l'API).
  el("github-form").onsubmit = async (ev) => {
    ev.preventDefault();
    const btn = el("github-btn"), err = el("github-error");
    err.classList.add("hidden");
    const name = el("g-name").value.trim(), token = el("g-token").value.trim();
    if (!name || !token) { err.textContent = "Un nom et un jeton sont nécessaires."; err.classList.remove("hidden"); return; }
    btn.disabled = true; btn.textContent = "Connexion…";
    try { await App().AddInstanceToken(name, "github", "", token); location.reload(); return; }
    catch (e) { err.textContent = cleanErr(e); err.classList.remove("hidden"); }
    finally { btn.disabled = false; btn.textContent = "Se connecter"; }
  };
  el("github-back").onclick = () => gotoProvider();
  { const cb = el("connect-back"); if (cb) cb.onclick = () => gotoProvider(); }

  // Mode de connexion Forgejo (#29, 1.19) : « creds » (identifiants → SituLab crée le jeton) ou
  // « token » (l'utilisateur colle un jeton qu'il a créé lui-même, sans confier son mot de passe).
  let fjMode = "creds";
  const setFjMode = (m) => {
    fjMode = m;
    const on = "var(--accent,#3b82f6)";
    if (el("fjm-creds")) { el("fjm-creds").style.background = m === "creds" ? on : "transparent"; el("fjm-creds").style.color = m === "creds" ? "#fff" : "var(--text)"; }
    if (el("fjm-token")) { el("fjm-token").style.background = m === "token" ? on : "transparent"; el("fjm-token").style.color = m === "token" ? "#fff" : "var(--text)"; }
    if (el("f-creds")) el("f-creds").hidden = m === "token";
    if (el("f-tokenwrap")) el("f-tokenwrap").hidden = m !== "token";
  };
  if (el("fjm-creds")) el("fjm-creds").onclick = () => setFjMode("creds");
  if (el("fjm-token")) el("fjm-token").onclick = () => setFjMode("token");

  async function doLogin(err, otp) {
    const name = el("f-name").value.trim(), url = el("f-url").value.trim();
    if (fjMode === "token") {
      const token = el("f-token").value.trim();
      if (!name || !url || !token) { err.textContent = "Nom, adresse et jeton sont nécessaires."; err.classList.remove("hidden"); return false; }
      await App().AddInstanceToken(name, "forgejo", url, token);
      return true;
    }
    const user = el("f-user").value.trim(), pass = el("f-pass").value;
    if (!name || !url || !user || !pass) { err.textContent = "Nom, adresse, identifiant et mot de passe sont nécessaires."; err.classList.remove("hidden"); return false; }
    // Jeton standard par défaut (moindre privilège) : le droit admin est accordé à la demande,
    // via « Activer l'administration » dans l'onglet Administration (v1.4.2).
    await App().AddInstanceLogin(name, url, user, pass, otp, false, !!(el("f-remember") && el("f-remember").checked));
    return true;
  }

  el("connect-form").onsubmit = async (ev) => {
    ev.preventDefault();
    const btn = el("connect-btn"), err = el("connect-error");
    err.classList.add("hidden");
    btn.disabled = true; btn.textContent = "Connexion…";
    try {
      if (await doLogin(err, "")) { location.reload(); return; }
    } catch (e) {
      // On ne bascule sur l'étape 2FA QUE si l'échec ressemble à un refus d'authentification ou mentionne
      // explicitement un code (le compte a peut-être une 2FA). Pour une autre erreur — réseau, URL
      // invalide, instance en double — on AFFICHE le message sur l'écran de connexion au lieu de déposer
      // l'utilisateur en silence sur un écran 2FA muet qui ne résoudra rien.
      const msg = cleanErr(e);
      // La 2FA ne concerne QUE la connexion par identifiants : en mode « jeton collé », une erreur
      // n'ouvre jamais l'écran 2FA (le jeton porte déjà l'authentification).
      const maybe2FA = fjMode === "creds" && (isAuthError(e) || /2fa|otp|two.?factor|double.?facteur|\bcode\b/i.test(msg));
      if (maybe2FA) {
        obShow("2fa");
        ciOtp.clear();
        el("otp-err").classList.add("hidden");
        setTimeout(() => { const b = el("f-otp").querySelector("input"); if (b) b.focus(); }, 40);
      } else {
        err.textContent = msg; err.classList.remove("hidden");
      }
    } finally {
      btn.disabled = false; btn.textContent = "Se connecter";
    }
  };

  // Étape 2b — 2FA. On rejoue la connexion avec le code, identifiants inchangés.
  const submitOtp = async () => {
    const err = el("otp-err"); err.classList.add("hidden");
    const code = ciOtp.read();
    if (code.length < CODELEN) { err.textContent = "Entrez le code à 6 chiffres."; err.classList.remove("hidden"); return; }
    const btn = el("otp-btn"); btn.disabled = true; btn.textContent = "…";
    try {
      if (await doLogin(err, code)) { location.reload(); return; }
    } catch (e) {
      err.textContent = cleanErr(e); err.classList.remove("hidden"); ciOtp.clear();
    } finally { btn.disabled = false; btn.textContent = "Valider"; }
  };
  el("otp-btn").onclick = submitOtp;
  el("otp-back").onclick = () => { obShow("connect"); setTimeout(() => { const fn = el("f-name"); if (fn) fn.focus(); }, 40); };
  onOtpComplete = submitOtp; // validation auto dès 6 chiffres

  // Choix de l'écran de départ : accueil (fraîche install) ou directement la connexion.
  if (!pinDone) {
    welcome.hidden = false; card.hidden = true;
    el("welcome-start").onclick = () => {
      welcome.hidden = true; card.hidden = false;
      setupStepper(false); obShow("pin");
      setTimeout(() => { const b = el("f-pin").querySelector("input"); if (b) b.focus(); }, 40);
    };
  } else {
    welcome.hidden = true; card.hidden = false;
    gotoProvider();
  }

  addRevealToggle(el("connect-form"));
  addRevealToggle(el("github-form"));
}


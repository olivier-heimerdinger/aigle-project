<script>
  import { onMount } from 'svelte';
  import { api, ROLE_BADGES } from '../api.js';

  let { onClose = () => {}, onAccountUpdated = () => {} } = $props();

  let tab = $state('password'); // 'password' | 'totp'
  let loading = $state(true);
  let accountInfo = $state({
    user: {},
    totp_enabled: false,
    totp_verified_session: false,
    password_mode: 'both',
    allow_direct_password: true,
    allow_authelia_portal: true,
    authelia_settings_url: 'https://aigle-auth.heimerdinger.studio/settings',
    totp_session_minutes: 720,
  });

  // Profil
  let displayName = $state('');
  let profileMsg = $state('');

  // Mot de passe
  let currentPassword = $state('');
  let newPassword = $state('');
  let confirmPassword = $state('');
  let pwdStatus = $state({ type: '', text: '' });
  let savingPwd = $state(false);

  // 2FA TOTP
  let totpEnrollment = $state(null); // { secret, otpauth_uri, qr_svg }
  let totpVerifyCode = $state('');
  let totpDisableCode = $state('');
  let totpStatus = $state({ type: '', text: '' });
  let copiedSecret = $state(false);

  async function loadAccount() {
    loading = true;
    try {
      const res = await api.getAccount();
      accountInfo = res;
      displayName = res.user?.display_name || '';
    } catch (err) {
      pwdStatus = { type: 'error', text: err.message };
    } finally {
      loading = false;
    }
  }

  async function saveProfile(e) {
    e.preventDefault();
    profileMsg = '';
    try {
      await api.updateAccountProfile({ display_name: displayName });
      profileMsg = '✅ Nom d’affichage mis à jour.';
      await loadAccount();
      onAccountUpdated();
    } catch (err) {
      profileMsg = '❌ ' + err.message;
    }
  }

  async function handleChangePassword(e) {
    e.preventDefault();
    pwdStatus = { type: '', text: '' };
    if (newPassword.length < 8) {
      pwdStatus = { type: 'error', text: 'Le nouveau mot de passe doit comporter au moins 8 caractères.' };
      return;
    }
    if (newPassword !== confirmPassword) {
      pwdStatus = { type: 'error', text: 'La confirmation du nouveau mot de passe ne correspond pas.' };
      return;
    }
    savingPwd = true;
    try {
      const res = await api.changePassword({
        current_password: currentPassword,
        new_password: newPassword,
        confirm_password: confirmPassword,
      });
      pwdStatus = {
        type: 'success',
        text: res.message + (res.authelia_synced ? ' (Synchronisé à chaud avec Authelia SSO)' : ''),
      };
      currentPassword = '';
      newPassword = '';
      confirmPassword = '';
    } catch (err) {
      pwdStatus = { type: 'error', text: err.message };
    } finally {
      savingPwd = false;
    }
  }

  async function initTOTPSetup() {
    totpStatus = { type: '', text: '' };
    try {
      totpEnrollment = await api.setupTOTP();
      totpVerifyCode = '';
    } catch (err) {
      totpStatus = { type: 'error', text: err.message };
    }
  }

  async function confirmTOTPActivation(e) {
    e.preventDefault();
    totpStatus = { type: '', text: '' };
    try {
      const res = await api.activateTOTP(totpVerifyCode);
      totpStatus = { type: 'success', text: res.message };
      totpEnrollment = null;
      totpVerifyCode = '';
      await loadAccount();
      onAccountUpdated();
    } catch (err) {
      totpStatus = { type: 'error', text: err.message };
    }
  }

  async function handleDisableTOTP(e) {
    e.preventDefault();
    totpStatus = { type: '', text: '' };
    try {
      const res = await api.disableTOTP(totpDisableCode);
      totpStatus = { type: 'success', text: res.message };
      totpDisableCode = '';
      await loadAccount();
      onAccountUpdated();
    } catch (err) {
      totpStatus = { type: 'error', text: err.message };
    }
  }

  function copySecret(secret) {
    if (navigator?.clipboard) {
      navigator.clipboard.writeText(secret);
      copiedSecret = true;
      setTimeout(() => (copiedSecret = false), 2000);
    }
  }

  onMount(loadAccount);
</script>

<div class="fixed inset-0 bg-black/75 backdrop-blur-xs flex items-center justify-center z-50 p-4">
  <div class="bg-slate-900 border border-slate-700 rounded-2xl w-full max-w-2xl overflow-hidden shadow-2xl text-xs">
    <!-- En-tête -->
    <div class="px-6 py-4 bg-slate-950 border-b border-slate-800 flex items-center justify-between">
      <div class="flex items-center gap-3">
        <div class="w-10 h-10 rounded-xl bg-indigo-600/20 border border-indigo-500/40 flex items-center justify-center text-lg">
          👤
        </div>
        <div>
          <h3 class="text-base font-bold text-white">Mon Compte, Mot de Passe & Double Authentification (2FA)</h3>
          <p class="text-[11px] text-slate-400 font-mono">
            {accountInfo.user?.email} • Identifiant SSO : <span class="text-indigo-300">{accountInfo.user?.username}</span>
          </p>
        </div>
      </div>
      <button onclick={onClose} class="text-slate-400 hover:text-white text-base px-2 py-1 cursor-pointer">✕</button>
    </div>

    <!-- Onglets -->
    <div class="px-6 bg-slate-900 border-b border-slate-800 flex items-center gap-2">
      <button
        onclick={() => (tab = 'password')}
        class="px-4 py-3 font-semibold border-b-2 transition cursor-pointer {tab === 'password'
          ? 'border-indigo-500 text-indigo-400'
          : 'border-transparent text-slate-400 hover:text-white'}"
      >
        🔑 1. Profil & Changement de Mot de Passe
      </button>
      <button
        onclick={() => (tab = 'totp')}
        class="px-4 py-3 font-semibold border-b-2 transition cursor-pointer flex items-center gap-1.5 {tab === 'totp'
          ? 'border-emerald-500 text-emerald-400'
          : 'border-transparent text-slate-400 hover:text-white'}"
      >
        <span>📱 2. Double Authentification (2FA TOTP)</span>
        {#if accountInfo.totp_enabled}
          <span class="px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 text-[9px] font-mono">ACTIF</span>
        {:else}
          <span class="px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-300 text-[9px] font-mono">NON CONFIGURÉ</span>
        {/if}
      </button>
    </div>

    <div class="p-6 max-h-[78vh] overflow-y-auto space-y-6">
      {#if loading}
        <div class="text-center py-8 text-slate-400">Chargement des informations de sécurité du compte...</div>
      {:else if tab === 'password'}
        <!-- Modification du Nom Affiché -->
        <form onsubmit={saveProfile} class="p-4 rounded-xl bg-slate-950 border border-slate-800 space-y-3">
          <div class="flex items-center justify-between">
            <div class="font-semibold text-white">Identité & Nom d'affichage</div>
            <span class="px-2 py-0.5 rounded text-[10px] border {(ROLE_BADGES[accountInfo.user?.role] || ROLE_BADGES.member).class}">
              {(ROLE_BADGES[accountInfo.user?.role] || ROLE_BADGES.member).label}
            </span>
          </div>
          <div class="flex gap-2">
            <input
              type="text"
              required
              bind:value={displayName}
              placeholder="Nom complet affiché dans les projets"
              class="flex-1 px-3 py-2 rounded-lg bg-slate-900 border border-slate-700 text-white"
            />
            <button type="submit" class="px-4 py-2 rounded-lg bg-slate-800 hover:bg-slate-700 text-white font-medium cursor-pointer">
              Mettre à jour le nom
            </button>
          </div>
          {#if profileMsg}
            <div class="text-[11px] text-emerald-400">{profileMsg}</div>
          {/if}
        </form>

        <!-- Mode 2 (Par défaut) : Changement direct du mot de passe Argon2id dans Aigle (sans SMTP requis) -->
        {#if accountInfo.allow_direct_password}
          <form onsubmit={handleChangePassword} class="p-4 rounded-xl bg-slate-950 border border-indigo-500/30 space-y-3.5">
            <div class="flex items-center justify-between">
              <div>
                <h4 class="font-bold text-white text-sm">🔒 Changer mon mot de passe (Mode Direct Argon2id)</h4>
                <p class="text-[11px] text-slate-400">
                  Vérifie votre mot de passe actuel et met à jour votre empreinte cryptographique <code class="text-indigo-300">Argon2id</code> directement dans Authelia (<code class="text-slate-300">users_database.yml</code>) sans nécessiter de serveur SMTP.
                </p>
              </div>
              <span class="px-2 py-0.5 rounded bg-indigo-500/20 text-indigo-300 font-mono text-[10px] shrink-0">
                Argon2id Direct
              </span>
            </div>

            {#if pwdStatus.text}
              <div
                class="p-3 rounded-lg border text-xs {pwdStatus.type === 'success'
                  ? 'bg-emerald-950/50 border-emerald-500/40 text-emerald-300'
                  : 'bg-rose-950/50 border-rose-500/40 text-rose-300'}"
              >
                {pwdStatus.text}
              </div>
            {/if}

            <div>
              <label for="curr-pwd" class="block text-slate-400 mb-1">Mot de passe actuel</label>
              <input
                id="curr-pwd"
                type="password"
                required
                bind:value={currentPassword}
                placeholder="Saisissez votre mot de passe actuel"
                class="w-full px-3 py-2 rounded-lg bg-slate-900 border border-slate-700 text-white font-mono"
              />
            </div>

            <div class="grid grid-cols-1 md:grid-cols-2 gap-3">
              <div>
                <label for="new-pwd" class="block text-slate-400 mb-1">Nouveau mot de passe (min. 8 caract.)</label>
                <input
                  id="new-pwd"
                  type="password"
                  required
                  minlength="8"
                  bind:value={newPassword}
                  placeholder="Nouveau mot de passe fort"
                  class="w-full px-3 py-2 rounded-lg bg-slate-900 border border-slate-700 text-white font-mono"
                />
              </div>
              <div>
                <label for="conf-pwd" class="block text-slate-400 mb-1">Confirmer le nouveau mot de passe</label>
                <input
                  id="conf-pwd"
                  type="password"
                  required
                  minlength="8"
                  bind:value={confirmPassword}
                  placeholder="Répétez le nouveau mot de passe"
                  class="w-full px-3 py-2 rounded-lg bg-slate-900 border border-slate-700 text-white font-mono"
                />
              </div>
            </div>

            <div class="flex justify-end pt-1">
              <button
                type="submit"
                disabled={savingPwd}
                class="px-4 py-2 rounded-xl bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 text-white font-semibold cursor-pointer"
              >
                {savingPwd ? 'Hachage Argon2id en cours...' : '🔑 Enregistrer le nouveau mot de passe'}
              </button>
            </div>
          </form>
        {/if}

        <!-- Mode 1 (Optionnel / Hybride par configuration) : Portail Natif Authelia + SMTP -->
        {#if accountInfo.allow_authelia_portal}
          <div class="p-4 rounded-xl bg-slate-950 border border-slate-800 flex items-center justify-between gap-4">
            <div class="space-y-1">
              <div class="font-semibold text-slate-200 flex items-center gap-2">
                <span>🌐 Portail Utilisateur Natif Authelia (Mode SMTP / WebAuthn)</span>
                <span class="px-1.5 py-0.5 rounded bg-slate-800 text-slate-400 font-mono text-[10px]">
                  AIGLE_PASSWORD_MODE={accountInfo.password_mode}
                </span>
              </div>
              <p class="text-[11px] text-slate-400">
                Lorsque le notifier SMTP d'Authelia est configuré dans <code class="text-slate-300">authelia/configuration.yml</code>, vous pouvez également gérer vos clés de sécurité WebAuthn/Passkeys et réinitialiser votre mot de passe par code e-mail sur le portail Authelia.
              </p>
            </div>
            <a
              href={accountInfo.authelia_settings_url}
              target="_blank"
              rel="noreferrer"
              class="px-3.5 py-2 rounded-xl bg-slate-800 hover:bg-slate-700 text-indigo-300 border border-slate-700 font-medium shrink-0"
            >
              Ouvrir Authelia /settings ↗
            </a>
          </div>
        {/if}
      {:else}
        <!-- ONGLET 2 : DOUBLE AUTHENTIFICATION 2FA TOTP (RFC 6238) -->
        <div class="space-y-4">
          <div class="p-4 rounded-xl bg-slate-950 border border-slate-800 flex items-start justify-between gap-4">
            <div class="space-y-1">
              <div class="text-sm font-bold text-white flex items-center gap-2">
                <span>📱 Authentificateur TOTP (Google Authenticator, Authy, Microsoft, Bitwarden)</span>
              </div>
              <p class="text-[11px] text-slate-400">
                Certains projets sensibles peuvent exiger la <strong>Double Authentification (2FA TOTP)</strong>. En activant votre TOTP ici, vous pourrez déverrouiller l'accès aux projets protégés avec un code temporaire à 6 chiffres (valable {Math.round((accountInfo.totp_session_minutes || 720) / 60)}h par session).
              </p>
            </div>
            <div class="shrink-0">
              {#if accountInfo.totp_enabled}
                <span class="px-3 py-1 rounded-full bg-emerald-500/15 text-emerald-300 border border-emerald-500/30 font-semibold text-xs">
                  ✅ 2FA TOTP Activé
                </span>
              {:else}
                <span class="px-3 py-1 rounded-full bg-amber-500/15 text-amber-300 border border-amber-500/30 font-semibold text-xs">
                  ⚠️ Non activé
                </span>
              {/if}
            </div>
          </div>

          {#if totpStatus.text}
            <div
              class="p-3 rounded-xl border text-xs {totpStatus.type === 'success'
                ? 'bg-emerald-950/50 border-emerald-500/40 text-emerald-300'
                : 'bg-rose-950/50 border-rose-500/40 text-rose-300'}"
            >
              {totpStatus.text}
            </div>
          {/if}

          {#if !accountInfo.totp_enabled && !totpEnrollment}
            <div class="p-5 rounded-xl bg-slate-950 border border-indigo-500/30 text-center space-y-3">
              <div class="text-2xl">🔐</div>
              <div class="text-sm font-semibold text-white">Configurer mon application d'authentification 2FA (TOTP)</div>
              <p class="text-xs text-slate-400 max-w-md mx-auto">
                Générez un QR Code TOTP standard (RFC 6238 - 6 chiffres / 30 secondes) et scannez-le avec votre téléphone pour sécuriser votre compte et accéder aux projets à haute confidentialité.
              </p>
              <button
                onclick={initTOTPSetup}
                class="px-5 py-2.5 rounded-xl bg-indigo-600 hover:bg-indigo-500 text-white font-semibold cursor-pointer"
              >
                📲 Générer mon QR Code 2FA TOTP
              </button>
            </div>
          {/if}

          {#if totpEnrollment}
            <form onsubmit={confirmTOTPActivation} class="p-5 rounded-xl bg-slate-950 border border-emerald-500/40 space-y-4">
              <div class="font-bold text-sm text-white">
                Étape 1 : Scannez ce QR Code avec votre application Authenticator
              </div>
              <div class="flex flex-col md:flex-row items-center gap-5">
                <div class="p-3 bg-white rounded-xl shadow-md shrink-0">
                  <img src={totpEnrollment.qr_svg} alt="QR Code TOTP" class="w-40 h-40" />
                </div>
                <div class="space-y-2.5 flex-1">
                  <p class="text-xs text-slate-300">
                    Ouvrez <strong>Google Authenticator</strong>, <strong>Authy</strong>, <strong>Microsoft Authenticator</strong> ou <strong>Bitwarden</strong> et scannez le QR Code ci-contre.
                  </p>
                  <div class="p-2.5 rounded-lg bg-slate-900 border border-slate-800 space-y-1">
                    <div class="text-[10px] text-slate-400 uppercase font-semibold">Clé secrète Base32 (saisie manuelle) :</div>
                    <div class="flex items-center justify-between gap-2">
                      <code class="font-mono text-xs text-amber-300 select-all break-all">{totpEnrollment.secret}</code>
                      <button
                        type="button"
                        onclick={() => copySecret(totpEnrollment.secret)}
                        class="px-2 py-1 rounded bg-slate-800 hover:bg-slate-700 text-[10px] text-slate-200 shrink-0 cursor-pointer"
                      >
                        {copiedSecret ? '✓ Copié' : 'Copier'}
                      </button>
                    </div>
                  </div>
                </div>
              </div>

              <div class="pt-3 border-t border-slate-800 space-y-2">
                <label for="totp-activate-code" class="block font-bold text-white">
                  Étape 2 : Saisissez le code à 6 chiffres affiché sur votre téléphone pour activer la 2FA
                </label>
                <div class="flex items-center gap-3">
                  <input
                    id="totp-activate-code"
                    type="text"
                    inputmode="numeric"
                    maxlength="6"
                    required
                    bind:value={totpVerifyCode}
                    placeholder="Ex: 482910"
                    class="w-44 px-3.5 py-2 rounded-xl bg-slate-900 border border-slate-700 text-white font-mono text-base tracking-widest text-center"
                  />
                  <button
                    type="submit"
                    class="px-5 py-2 rounded-xl bg-emerald-600 hover:bg-emerald-500 text-white font-semibold cursor-pointer"
                  >
                    ✅ Valider & Activer la 2FA
                  </button>
                  <button
                    type="button"
                    onclick={() => (totpEnrollment = null)}
                    class="px-3 py-2 rounded-xl bg-slate-800 text-slate-300 cursor-pointer"
                  >
                    Annuler
                  </button>
                </div>
              </div>
            </form>
          {/if}

          {#if accountInfo.totp_enabled && !totpEnrollment}
            <div class="p-4 rounded-xl bg-slate-950 border border-slate-800 space-y-3">
              <div class="flex items-center justify-between">
                <div>
                  <div class="font-semibold text-white">État de la session 2FA TOTP actuelle</div>
                  <div class="text-[11px] text-slate-400">
                    {accountInfo.totp_verified_session
                      ? '🟢 Votre session actuelle est déjà vérifiée en 2FA pour accéder aux projets protégés.'
                      : '🟡 Lors de l’ouverture d’un projet protégé par 2FA, votre code à 6 chiffres vous sera demandé.'}
                  </div>
                </div>
                <button
                  onclick={initTOTPSetup}
                  class="px-3 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 text-[11px] cursor-pointer"
                >
                  🔄 Régénérer un QR Code
                </button>
              </div>

              <form onsubmit={handleDisableTOTP} class="pt-3 border-t border-slate-800 flex items-center justify-between gap-3">
                <span class="text-[11px] text-slate-400">Désactiver la 2FA TOTP (nécessite un code à 6 chiffres valide) :</span>
                <div class="flex items-center gap-2">
                  <input
                    type="text"
                    inputmode="numeric"
                    maxlength="6"
                    required
                    bind:value={totpDisableCode}
                    placeholder="Code 6 chiffres"
                    class="w-32 px-2.5 py-1.5 rounded-lg bg-slate-900 border border-slate-700 text-white font-mono text-center"
                  />
                  <button
                    type="submit"
                    class="px-3 py-1.5 rounded-lg bg-rose-950/60 hover:bg-rose-900/60 text-rose-300 border border-rose-500/30 cursor-pointer"
                  >
                    Désactiver 2FA
                  </button>
                </div>
              </form>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <div class="px-6 py-3 bg-slate-950 border-t border-slate-800 flex justify-end">
      <button onclick={onClose} class="px-4 py-1.5 rounded-lg bg-slate-800 hover:bg-slate-700 text-slate-200 cursor-pointer">
        Fermer
      </button>
    </div>
  </div>
</div>


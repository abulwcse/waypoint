import { useEffect, useRef } from 'react'
import { loadGoogleIdentity } from './googleIdentity.js'

// GoogleSignIn renders Google's own "Sign in with Google" button. Google
// mounts real DOM into buttonRef itself (an iframe), so this stays a thin
// wrapper rather than a styled button — onCredential fires once the user
// completes sign-in, with the ID token for the server to verify.
export default function GoogleSignIn({ clientId, onCredential }) {
  const buttonRef = useRef(null)

  useEffect(() => {
    let cancelled = false
    loadGoogleIdentity()
      .then((google) => {
        if (cancelled || !buttonRef.current) return
        google.accounts.id.initialize({
          client_id: clientId,
          callback: (response) => onCredential(response.credential),
        })
        google.accounts.id.renderButton(buttonRef.current, {
          theme: 'outline',
          size: 'medium',
          type: 'standard',
        })
      })
      .catch(() => {}) // sign-in is optional; don't block the app if Google's script fails to load
    return () => {
      cancelled = true
    }
  }, [clientId, onCredential])

  return <div className="google-signin" ref={buttonRef} />
}

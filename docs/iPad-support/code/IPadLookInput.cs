using System.Runtime.InteropServices;
using UnityEngine;

namespace MHS.InputSystem
{
    /// <summary>
    /// iPad two-finger trackpad look fallback.
    ///
    /// iPadOS Safari has no Pointer Lock API, so mouse-look via &lt;Pointer&gt;/delta
    /// pins at the screen edge. Instead, we listen for wheel events (two-finger
    /// trackpad drag), accumulate deltas in JS, and drain them here each frame.
    ///
    /// Desktop and Editor return IsActive = false; callers fall through to the
    /// normal Input System path.
    /// </summary>
    public static class IPadLookInput
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        [DllImport("__Internal")] private static extern int iPadInput_Init();
        [DllImport("__Internal")] private static extern float iPadInput_DrainDeltaX();
        [DllImport("__Internal")] private static extern float iPadInput_DrainDeltaY();
#endif

        public static bool IsActive { get; private set; }

        // Matches the ScaleVector2(0.05) processor on the desktop Look binding.
        public static float SensitivityX = 0.05f;
        public static float SensitivityY = 0.05f;

        // Defaults assume macOS/iPadOS natural scrolling is on (the system default):
        // a two-finger swipe up fires deltaY < 0, which we negate so "swipe up"
        // looks up. If a tester has natural scrolling off, flip these.
        public static bool InvertX = true;
        public static bool InvertY = true;

        [RuntimeInitializeOnLoadMethod(RuntimeInitializeLoadType.BeforeSceneLoad)]
        private static void Init()
        {
#if UNITY_WEBGL && !UNITY_EDITOR
            try
            {
                IsActive = iPadInput_Init() == 1;
                if (IsActive)
                    Debug.Log("IPadLookInput: iPad detected — using two-finger trackpad wheel events for look");
            }
            catch (System.Exception e)
            {
                IsActive = false;
                Debug.LogWarning("IPadLookInput: init failed: " + e.Message);
            }
#else
            IsActive = false;
#endif
        }

        /// <summary>
        /// Returns and zeros the accumulated two-finger scroll delta, scaled and
        /// sign-corrected to match the desktop Look action's output shape.
        /// Returns Vector2.zero if iPad mode is not active.
        /// </summary>
        public static Vector2 DrainLookDelta()
        {
#if UNITY_WEBGL && !UNITY_EDITOR
            if (!IsActive) return Vector2.zero;
            float dx = iPadInput_DrainDeltaX() * SensitivityX;
            float dy = iPadInput_DrainDeltaY() * SensitivityY;
            if (InvertX) dx = -dx;
            if (InvertY) dy = -dy;
            return new Vector2(dx, dy);
#else
            return Vector2.zero;
#endif
        }
    }
}

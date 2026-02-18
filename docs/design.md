# Qrun Design v0

## Overview

Qrun is a management layer for Qlab, used to provide higher-level abstractions and make building complex Qlab configurations easier to get right.

Qrun is intended to simplify management, but all state is stored in Qlab, and there is an invariant that the show is capable of running completely with just Qlab, not requiring Qrun (e.g. the "Go" button is wired to Qlab, not Qrun). However, Qrun does provide a show run friendlier UI, i.e. showing position in the show, timers on blocks, and relaying commands to Qlab. The exception to this is panic/reset, where Qlab doesn't have the functionality to fully reset state. When Qrun is reset to a particular location in the timeline, it fires the minimum set of Qlab cues necessary to activate the correct state. The reset operation has a default and configurable fade time.

Qrun acts a compiler of complex state to lower-level Qlab cues, using Qlab Groups, Note fields, and Memo cues to store extra structured information as necessary. When Qrun starts, it reads the cue list from Qlab, extracts the higher-level constructs, and ensures that the compilation results in the output cue state, ensuring that everything is in sync. If a mismatch is detected, the user is prompted to confirm that the reconstructed Qrun state will override Qlab cues.

## Architecture

* _Qrun Proxy_: A service that runs one instance, probably on the same computer as Qlab. Speaks OSC to Qlab, then exports a REST/SSE interface to Qrun clients. Maintains state for information that can't easily be retrieved from Qlab, e.g. lighting dashboard state.
* _Qrun Client_: A user-facing service, possibly running on a different computer than Qlab, speaking to the Qlab Proxy for both read and write.

## Data Model

* _Block_: A representation state for some theater media, e.g. lights at a certain direction/intensity/color, a video looping, a sound effect playing.
* _Signal_: An event emitted by a block, e.g. "video start", "video fade in complete", "audio fade out start"
* _Hook_: A point at a block that might wait for a signal, e.g. "start video" or "fade out video loop". Some hooks may be optional (e.g. "fade out non-looping video 3s before end"), while some may be required (e.g. "fade out looping video over 3s"). If a required end hook isn't connected to a signal, that block will continue on to infinity, and the UI will represent this by extending them all the way to the bottom of the timeline and marking their end specially (e.g. wavy line after all other blocks complete).
* _Timeline_: The overall UI metaphor. Time runs top to bottom. Blocks have a start, end, and hooks. Vertical height is not to scale with time -- it's one row per event (combination of signal and hooks).
* _Track_: A column in the timeline. Only one block may be in each track at any time point in the timeline. Used as both a conceptual separation ("video track", "video wipe overlay track") and as layering definition (tracks to the right go in front of tracks to the left, whether it be video alpha stacking or resolving conflicts in lighting cues). Tracks may have mix modes, but generally use the expected mix mode, e.g. alpha overlay for video, per-instrument lighting overrides, and additive mixing for audio. Lighting instruments are split into position/color/intensity, and overrides only occur within those settings, not at the full instrument level. Overridden blocks have a visual indication for partial/full override to make it obvious to the user.
* _Connection_: A link between a signal and one or more hooks. Signified in the UI by them being on the same grid row, implying that they're temporally connected.
* _Cue_: A special block type that requires a human "Go". It lives in a special Cue track so there can only ever be one active. It emits a Go signal.
* _Delay_: A special block type that implements a fixed delay. It has an optional start hook (this is a common pattern -- without it, it just follows the previous block in the track), and emits a completion signal. This is used for pre and post waits.
* _Template_: A special block type that lives outside the timeline. It has all the properties of a normal block, but is never activated directly.
* _Instance_: A block that derives all its properties from a template, but is placed in the timeline. Any change to the template is reflected in all instances of that template. Instance overrides are handled via the track layering logic.

Possible future features:

* _Track Groups_: Conceptual groupings of trackings that can be collapsed/expanded

## UIs

In all UIs:
* Primary view is timeline
  * At any time, one row in the timeline is clearly highlighted as the current state
* Other views (light position aim, light color/intensity selection) exist

* Web UI: Zero framework, needs to run offline and be incredibly fast and responsive. Emphasizes performance and ease of use (e.g. highly legible font, high contrast). Gets realtime status from Qrun Proxy SSE feed, sends updates via REST interface.
* Framebuffer UI: Designed to run from a Raspberry Pi, driving a rugged monitor (e.g. a Blackmagic SmartView). Similar interface to Web UI (which necessitates keeping the web features in use relatively simple). Input is via MIDI infinite encoder device and StreamDeck Studio. The encoders and StreamDeck buttons are dynamic and mode-based, so selecting timeline navigation with the buttons makes the encoders perform timeline navigation, while switching to light editing mode with the buttons, then selecting a group of lights, switches the encoders to color selection mode. Visual feedback on the screen combined with LED ring coloring around the encoders is used to indicate encoder functionality at any given time.

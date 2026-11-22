#!/usr/bin/env python3
"""Explorar el mando BLE de la Sunday Box (sin la caja).

Busca el mando por nombre, se conecta, lista servicios y características,
se suscribe a todo lo que notifica y escribe por pantalla lo que llega al
pulsar botones. También puede encender la luz del mando.

UUID sacados de la app original de Sunday (ver docs/LESSONS.md).

    python3 -m venv venv && venv/bin/pip install bleak
    venv/bin/python sunday_remote.py               # escuchar 120 s
    venv/bin/python sunday_remote.py --led 255,0,0 --efecto 1
"""
import argparse
import asyncio
import time

from bleak import BleakClient, BleakScanner

NOMBRES = ("Sunday", "4MOD 45 BLE")


def ble(short):
    return f"0000{short}-0000-1000-8000-00805f9b34fb"


def ti(short):
    return f"f000{short}-0451-4000-b000-000000000000"


TECLAS_SVC = ble("ff00")        # "hidService" en la app (no es el 0x1812 estándar)
TECLAS_CHR = ble("2a4d")        # report: 1 byte, u 8 bytes con la tecla en [2]
BATERIA = ble("2a19")
FIRMWARE = ble("2a26")
VOZ_ON_OFF = ti("b001")         # 0x04 = empieza voz, otro valor = para
VOZ_AUDIO = ti("b002")
LED_COLOR = ble("fed1")         # 3 bytes, 0..100 cada uno (orden B, G, R)
LED_PERIODO = ble("fed2")       # 1 byte, periodo en décimas de segundo
LED_EFECTO = ble("fed3")        # 1 byte


def tecla(data: bytes):
    if len(data) == 1:
        return data[0]
    if len(data) == 8:
        return data[2]
    return None


async def buscar(timeout):
    print(f"Buscando {NOMBRES} durante {timeout} s…")
    dev = await BleakScanner.find_device_by_filter(
        lambda d, ad: (d.name or ad.local_name or "") in NOMBRES, timeout=timeout)
    if not dev:
        raise SystemExit("No aparece el mando. Pulsa un botón (o mantén Mic) y repite.")
    print(f"Encontrado: {dev.name} {dev.address}")
    return dev


async def diagnostico(c):
    """Lee cada característica legible y prueba lo que suele despertar un HID."""
    try:
        print("emparejar:", await c.pair())
    except Exception as e:  # noqa: BLE001
        print(f"emparejar: falló ({e})")
    for s in c.services:
        for ch in s.characteristics:
            if "read" in ch.properties:
                try:
                    v = bytes(await c.read_gatt_char(ch))
                    print(f"  leer h={ch.handle:#06x} {ch.uuid[:8]}: {v.hex(' ')}")
                except Exception as e:  # noqa: BLE001
                    print(f"  leer h={ch.handle:#06x} {ch.uuid[:8]}: error {e}")
            for d in ch.descriptors:
                try:
                    v = bytes(await c.read_gatt_descriptor(d.handle))
                    print(f"    desc {d.uuid[:8]} de h={ch.handle:#06x}: {v.hex(' ')}")
                except Exception as e:  # noqa: BLE001
                    print(f"    desc {d.uuid[:8]} de h={ch.handle:#06x}: error {e}")
    # Protocol Mode = Report (1) y Control Point = Exit Suspend (1), como un HID normal.
    for uuid, val in ((ble("2a4e"), b"\x01"), (ble("2a4c"), b"\x01")):
        try:
            await c.write_gatt_char(uuid, val, response=False)
            print(f"escrito {uuid[:8]} = {val.hex()}")
        except Exception as e:  # noqa: BLE001
            print(f"escribir {uuid[:8]}: {e}")


async def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--segundos", type=int, default=120, help="tiempo escuchando")
    ap.add_argument("--buscar", type=int, default=45, help="segundos buscando el mando")
    ap.add_argument("--led", help="color R,G,B (0-255) para la luz del mando")
    ap.add_argument("--efecto", type=int, default=1, help="efecto de la luz (byte)")
    ap.add_argument("--periodo", type=int, default=1000, help="periodo de la luz en ms")
    ap.add_argument("--diag", action="store_true",
                    help="leer todo lo legible, emparejar y activar el modo informe del HID propio")
    a = ap.parse_args()

    dev = await buscar(a.buscar)
    t_con = time.monotonic()

    def desconectado(_):
        print(f"*** el mando se ha desconectado a los {time.monotonic() - t_con:.1f} s de conectar")

    async with BleakClient(dev, disconnected_callback=desconectado) as c:
        print("Conectado. Servicios:")
        notificables = []
        for s in c.services:
            print(f"  {s.uuid}  {s.description}")
            for ch in s.characteristics:
                print(f"    {ch.uuid}  h={ch.handle:#06x}  {','.join(ch.properties)}")
                if "notify" in ch.properties or "indicate" in ch.properties:
                    notificables.append(ch)

        for uuid, nombre in ((BATERIA, "batería"), (FIRMWARE, "firmware")):
            try:
                v = await c.read_gatt_char(uuid)
                print(f"{nombre}: {v[0] if nombre == 'batería' else v.decode(errors='replace')}")
            except Exception as e:  # noqa: BLE001
                print(f"{nombre}: no se pudo leer ({e})")

        if a.diag:
            await diagnostico(c)

        if a.led:
            r, g, b = (int(x) for x in a.led.split(","))
            await c.write_gatt_char(LED_COLOR, bytes(v * 100 // 255 for v in (b, g, r)), response=True)
            await c.write_gatt_char(LED_PERIODO, bytes([a.periodo // 100]), response=True)
            await c.write_gatt_char(LED_EFECTO, bytes([a.efecto]), response=True)
            print(f"Luz: rgb=({r},{g},{b}) efecto={a.efecto} periodo={a.periodo} ms")

        t0 = time.monotonic()

        def handler(ch):
            def f(_, data: bytearray):
                extra = ""
                if ch.uuid == TECLAS_CHR:
                    extra = f"  tecla={tecla(bytes(data))}"
                if ch.uuid == VOZ_AUDIO:
                    extra = f"  ({len(data)} bytes de audio)"
                    data = data[:4]
                print(f"{time.monotonic() - t0:7.2f}s  h={ch.handle:#06x} {ch.uuid[:8]}  {bytes(data).hex(' ')}{extra}")
            return f

        for ch in notificables:
            try:
                await c.start_notify(ch, handler(ch))
                print(f"suscrito a {ch.uuid} (h={ch.handle:#06x})")
            except Exception as e:  # noqa: BLE001
                print(f"no se pudo suscribir a {ch.uuid}: {e}")

        print(f"Escuchando {a.segundos} s. Pulsa botones del mando…")
        await asyncio.sleep(a.segundos)


if __name__ == "__main__":
    asyncio.run(main())

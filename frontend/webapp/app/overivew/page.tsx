'use client';

import { useEffect } from 'react';
import { useRouter } from 'next/navigation';

/** Typo handler: `/overivew` → `/overview` (static export has no server redirects). */
export default function OverivewTypoRedirect() {
  const router = useRouter();

  useEffect(() => {
    router.replace('/overview');
  }, [router]);

  return null;
}

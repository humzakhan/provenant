import { defineConfig } from 'astro/config';
import mdx from '@astrojs/mdx';
import starlight from '@astrojs/starlight';

export default defineConfig({
  integrations: [
    starlight({
      title: 'Provenant Docs',
      customCss: ['./src/styles/provenant.css'],
      sidebar: [
        {
          label: 'Orientation',
          items: [
            { label: 'Home', link: '/' },
            { label: 'Guarantees and Limits', link: '/guarantees-and-limits/' },
          ],
        },
        {
          label: 'Current Artifact Quickstart',
          items: [{ label: 'Run Current Artifacts', link: '/quickstart/current-artifacts/' }],
        },
        {
          label: 'Concepts',
          items: [
            { label: 'Provenance Model', link: '/concepts/provenance-model/' },
            { label: 'Modes', link: '/concepts/modes/' },
          ],
        },
        {
          label: 'Guides',
          items: [
            { label: 'Review Audit Evidence', link: '/guides/review-audit-evidence/' },
            { label: 'Integrate the Go Library', link: '/guides/integrate-go-library/' },
            { label: 'Extend Source Tools', link: '/guides/extend-source-tools/' },
          ],
        },
        {
          label: 'Reference',
          items: [
            { label: 'Status Matrix', link: '/reference/status-matrix/' },
            { label: 'Current Surfaces', link: '/reference/current-surfaces/' },
          ],
        },
      ],
    }),
    mdx(),
  ],
});
